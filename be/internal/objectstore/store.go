// Package objectstore owns private, path-style S3 access. Credentials never reach clients.
package objectstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Store struct {
	Client         *minio.Client
	Bucket, Prefix string
}

func FromEnv() (*Store, error) {
	endpoint := os.Getenv("S3_ENDPOINT")
	if endpoint == "" {
		return nil, nil
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || (u.Path != "" && u.Path != "/") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("S3_ENDPOINT must be an http(s) origin without bucket/path")
	}
	bucket, access, secret := os.Getenv("S3_BUCKET"), os.Getenv("S3_ACCESS_KEY"), os.Getenv("S3_SECRET_KEY")
	if bucket == "" || access == "" || secret == "" {
		return nil, fmt.Errorf("S3_BUCKET, S3_ACCESS_KEY and S3_SECRET_KEY required when S3_ENDPOINT is set")
	}
	prefix := strings.Trim(os.Getenv("S3_PREFIX"), "/")
	if prefix == "" {
		prefix = "prod"
	}
	for _, part := range strings.Split(prefix, "/") {
		if part == "." || part == ".." {
			return nil, fmt.Errorf("invalid S3_PREFIX")
		}
	}
	client, err := minio.New(u.Host, &minio.Options{Creds: credentials.NewStaticV4(access, secret, ""), Secure: u.Scheme == "https", Region: os.Getenv("S3_REGION"), BucketLookup: minio.BucketLookupPath})
	if err != nil {
		return nil, err
	}
	return &Store{client, bucket, prefix}, nil
}
func (s *Store) Key(id, kind, hash, ext string) string {
	return path.Join(s.Prefix, "recordings", id, kind, hash+ext)
}
func HashFile(filename string) (string, int64, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), n, err
}
func (s *Store) Upload(ctx context.Context, key, filename, mime, hash string) error {
	_, err := s.Client.FPutObject(ctx, s.Bucket, key, filename, minio.PutObjectOptions{ContentType: mime, UserMetadata: map[string]string{"sha256": hash}, CacheControl: "private, max-age=31536000, immutable"})
	if err != nil {
		return err
	}
	stat, err := s.Client.StatObject(ctx, s.Bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return err
	}
	local, err := os.Stat(filename)
	if err != nil {
		return err
	}
	if stat.Size != local.Size() || stat.UserMetadata["Sha256"] != hash && stat.UserMetadata["sha256"] != hash {
		return fmt.Errorf("object verification failed")
	}
	return nil
}

// MinIO Object is a lazy io.ReadSeeker; ServeContent handles Range, HEAD,
// If-Range and invalid ranges without buffering an entire MP3.
func (s *Store) Serve(w http.ResponseWriter, r *http.Request, key, mime string) {
	obj, err := s.Client.GetObject(r.Context(), s.Bucket, key, minio.GetObjectOptions{})
	if err != nil {
		http.Error(w, "Media storage unavailable", 503)
		return
	}
	defer obj.Close()
	stat, err := obj.Stat()
	if err != nil {
		http.Error(w, "Media storage unavailable", 503)
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("ETag", `"`+stat.ETag+`"`)
	w.Header().Set("Cache-Control", "private, max-age=0, must-revalidate")
	http.ServeContent(w, r, path.Base(key), stat.LastModified, obj)
}
