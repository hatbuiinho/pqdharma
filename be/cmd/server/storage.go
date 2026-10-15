package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"dharma-player/internal/objectstore"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func uploadAsset(ctx context.Context, db *pgxpool.Pool, store *objectstore.Store, id, kind, filename, mime, ext string) error {
	hash, size, err := objectstore.HashFile(filename)
	if err != nil {
		return err
	}
	key := store.Key(id, kind, hash, ext)
	var old string
	err = db.QueryRow(ctx, "SELECT sha256 FROM recording_assets WHERE recording_id=$1 AND kind=$2", id, kind).Scan(&old)
	if err == nil && old == hash {
		return nil
	}
	if err != nil && err != pgx.ErrNoRows {
		return err
	}
	if err = store.Upload(ctx, key, filename, mime, hash); err != nil {
		return err
	}
	_, err = db.Exec(ctx, `INSERT INTO recording_assets(recording_id,kind,object_key,content_type,size_bytes,sha256) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(recording_id,kind) DO UPDATE SET object_key=EXCLUDED.object_key,content_type=EXCLUDED.content_type,size_bytes=EXCLUDED.size_bytes,sha256=EXCLUDED.sha256,updated_at=now()`, id, kind, key, mime, size, hash)
	return err
}

func registerStoredMedia(mux *http.ServeMux, db *pgxpool.Pool, store *objectstore.Store) {
	mux.HandleFunc("GET /api/recordings/{id}/thumbnail", func(w http.ResponseWriter, r *http.Request) {
		var key, mime string
		err := db.QueryRow(r.Context(), "SELECT object_key,content_type FROM recording_assets WHERE recording_id=$1 AND kind='thumbnails'", r.PathValue("id")).Scan(&key, &mime)
		if err == pgx.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "Storage unavailable", 503)
			return
		}
		store.Serve(w, r, key, mime)
	})
}

// Reconciliation is retryable after restart. Local files are deliberately retained
// until an operator verifies migration; source objects are never auto-expired.
func syncStorage(ctx context.Context, db *pgxpool.Pool, s *objectstore.Store, dir, sample string) error {
	rows, err := db.Query(ctx, "SELECT id,audio_file,status FROM recordings ORDER BY id")
	if err != nil {
		return err
	}
	type recording struct{ id, file, status string }
	var records []recording
	for rows.Next() {
		var r recording
		if err = rows.Scan(&r.id, &r.file, &r.status); err != nil {
			rows.Close()
			return err
		}
		records = append(records, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	var failures int
	for _, r := range records {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		filename := filepath.Join(dir, filepath.Base(r.file))
		if r.id == "sample" {
			filename = filepath.Join(filepath.Dir(sample), filepath.Base(r.file))
		}
		// Never race the downloader's publication of audio and its source sidecars.
		var active bool
		if err = db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM processing_jobs WHERE recording_id=$1 AND status NOT IN ('ready','failed'))", r.id).Scan(&active); err != nil {
			return err
		}
		if active {
			continue
		}
		if _, e := os.Stat(filename); os.IsNotExist(e) {
			var stored bool
			if e = db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM recording_assets WHERE recording_id=$1 AND kind='audio')", r.id).Scan(&stored); e != nil {
				return e
			}
			if !stored {
				log.Printf("storage sync %s: audio missing locally and no stored object", r.id)
				failures++
			}
		}
		assets := []struct{ kind, file, mime, ext string }{
			{"audio", filename, "audio/mpeg", ".mp3"},
			{"source", filename[:len(filename)-len(filepath.Ext(filename))] + ".source.json", "application/json", ".json"},
			{"captions", filename[:len(filename)-len(filepath.Ext(filename))] + ".captions.json3", "application/json", ".json3"},
			{"thumbnails", filename[:len(filename)-len(filepath.Ext(filename))] + ".thumbnail.jpg", "image/jpeg", ".jpg"},
		}
		if r.id == "sample" {
			assets = append(assets, struct{ kind, file, mime, ext string }{"captions", filepath.Join(filepath.Dir(sample), "captions.en.json3"), "application/json", ".json3"})
		}
		for _, a := range assets {
			if _, e := os.Stat(a.file); os.IsNotExist(e) {
				continue
			}
			if e := uploadAsset(ctx, db, s, r.id, a.kind, a.file, a.mime, a.ext); e != nil {
				log.Printf("storage sync %s/%s: %v", r.id, a.kind, e)
				failures++
			}
		}
		if r.status == "ready" || r.status == "draft" {
			if e := exportTranscript(ctx, db, s, r.id); e != nil {
				log.Printf("storage export %s: %v", r.id, e)
				failures++
			}
		}
	}
	if failures > 0 {
		return fmt.Errorf("%d storage synchronization failures; files retained", failures)
	}
	return nil
}

func exportTranscript(ctx context.Context, db *pgxpool.Pool, s *objectstore.Store, id string) error {
	tx, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	transcript, err := readTranscript(ctx, tx, id)
	if err != nil {
		return err
	}
	data, err := json.Marshal(transcript)
	if err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	var old string
	err = db.QueryRow(ctx, "SELECT fingerprint FROM storage_exports WHERE recording_id=$1", id).Scan(&old)
	if err == nil && old == hash {
		return nil
	}
	if err != nil && err != pgx.ErrNoRows {
		return err
	}
	key := s.Key(id, "exports", fmt.Sprintf("transcript-r%d-%s", transcript.Revision, hash), ".json")
	_, err = s.Client.PutObject(ctx, s.Bucket, key, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{ContentType: "application/json", UserMetadata: map[string]string{"sha256": hash}})
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `INSERT INTO storage_exports(recording_id,fingerprint,object_key) VALUES($1,$2,$3) ON CONFLICT(recording_id) DO UPDATE SET fingerprint=EXCLUDED.fingerprint,object_key=EXCLUDED.object_key,updated_at=now()`, id, hash, key)
	return err
}
func runStorageSync(ctx context.Context, db *pgxpool.Pool, s *objectstore.Store, dir, sample string) {
	for {
		c, cancel := context.WithTimeout(ctx, 10*time.Minute)
		err := syncStorage(c, db, s, dir, sample)
		cancel()
		if err != nil && ctx.Err() == nil {
			log.Print(err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Minute):
		}
	}
}
