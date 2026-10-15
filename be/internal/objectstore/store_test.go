package objectstore

import (
	"context"
	"github.com/minio/minio-go/v7"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestConfiguration(t *testing.T) {
	for _, endpoint := range []string{"https://host/bucket", "https://user:pass@host", "ftp://host", "https://host?secret=value"} {
		t.Setenv("S3_ENDPOINT", endpoint)
		if _, err := FromEnv(); err == nil {
			t.Fatalf("accepted invalid endpoint %s", endpoint)
		}
	}
	t.Setenv("S3_ENDPOINT", "")
	s, err := FromEnv()
	if err != nil || s != nil {
		t.Fatal("local mode should not need credentials")
	}
}

// Opt-in: use a disposable bucket or isolated, cleaned-up objects in a configured bucket.
func TestMinIOStream(t *testing.T) {
	endpoint := os.Getenv("TEST_S3_ENDPOINT")
	existing := os.Getenv("TEST_S3_EXISTING_BUCKET") == "1"
	if existing {
		endpoint = os.Getenv("S3_ENDPOINT")
	}
	if endpoint == "" {
		t.Skip("set TEST_S3_ENDPOINT for real MinIO streaming tests")
	}
	t.Setenv("S3_ENDPOINT", endpoint)
	if !existing {
		t.Setenv("S3_BUCKET", "pqd-test-"+time.Now().Format("150405000000"))
		t.Setenv("S3_PREFIX", "test")
		t.Setenv("S3_ACCESS_KEY", "pqdtest")
		t.Setenv("S3_SECRET_KEY", "pqd-test-local-only")
		t.Setenv("S3_REGION", "us-east-1")
	}
	s, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if !existing {
		if err = s.Client.MakeBucket(ctx, s.Bucket, minio.MakeBucketOptions{}); err != nil {
			t.Fatal(err)
		}
		defer s.Client.RemoveBucket(ctx, s.Bucket)
	}
	file := t.TempDir() + "/audio.mp3"
	payload := "0123456789abcdef"
	os.WriteFile(file, []byte(payload), 0600)
	hash, _, err := HashFile(file)
	if err != nil {
		t.Fatal(err)
	}
	key := s.Key("storage-check-"+time.Now().Format("20060102T150405.000000000"), "audio", hash, ".mp3")
	if err = s.Upload(ctx, key, file, "audio/mpeg", hash); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Client.RemoveObject(ctx, s.Bucket, key, minio.RemoveObjectOptions{}); err != nil {
			t.Errorf("test object cleanup failed (%s): %v", key, err)
		}
	}()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { s.Serve(w, r, key, "audio/mpeg") }))
	defer server.Close()
	for _, tc := range []struct {
		method, rangeValue, body string
		status                   int
	}{
		{"GET", "", payload, 200}, {"GET", "bytes=3-6", "3456", 206}, {"GET", "bytes=-4", "cdef", 206},
		{"GET", "bytes=99-", "", 416}, {"HEAD", "", "", 200},
	} {
		req, _ := http.NewRequest(tc.method, server.URL, nil)
		if tc.rangeValue != "" {
			req.Header.Set("Range", tc.rangeValue)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != tc.status {
			t.Fatalf("%s %s: %d %s", tc.method, tc.rangeValue, resp.StatusCode, body)
		}
		if tc.status != 416 && string(body) != tc.body {
			t.Fatalf("wrong range body %q", body)
		}
	}
	req, _ := http.NewRequest("GET", server.URL, nil)
	req.Header.Set("Range", "bytes=1-2")
	req.Header.Set("If-Range", `"stale"`)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || string(body) != payload {
		t.Fatal("If-Range must fall back to full object")
	}
	// The bucket remains private.
	resp, err = http.Get(endpoint + "/" + s.Bucket + "/" + key)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("anonymous object access returned %d", resp.StatusCode)
	}
}
