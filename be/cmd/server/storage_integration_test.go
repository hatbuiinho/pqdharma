package main

import (
	"context"
	"dharma-player/internal/objectstore"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func testStorageDatabase(t *testing.T, db *pgxpool.Pool) {
	if os.Getenv("TEST_S3_EXISTING_BUCKET") != "1" {
		t.Skip("set TEST_S3_EXISTING_BUCKET=1 for real storage checks")
	}
	s, err := objectstore.FromEnv()
	if err != nil || s == nil {
		t.Fatalf("storage configuration: %v", err)
	}
	ctx := context.Background()
	id := "storage-check-" + newID()
	_, err = db.Exec(ctx, "INSERT INTO sutras VALUES($1,'storage test','en')", id)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, "DELETE FROM recordings WHERE id=$1", id)
	_, err = db.Exec(ctx, "INSERT INTO recordings(id,sutra_id,title,audio_file,source_url,status) VALUES($1,$1,'storage test','audio.mp3','','ready')", id)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	defer func() {
		for _, key := range keys {
			if e := s.Client.RemoveObject(ctx, s.Bucket, key, minio.RemoveObjectOptions{}); e != nil {
				t.Errorf("cleanup %s: %v", key, e)
			}
		}
	}()
	filename := filepath.Join(t.TempDir(), "audio.mp3")
	os.WriteFile(filename, []byte("audio test"), 0600)
	if err = uploadAsset(ctx, db, s, id, "audio", filename, "audio/mpeg", ".mp3"); err != nil {
		t.Fatal(err)
	}
	var key string
	if err = db.QueryRow(ctx, "SELECT object_key FROM recording_assets WHERE recording_id=$1", id).Scan(&key); err != nil {
		t.Fatal(err)
	}
	keys = append(keys, key)
	// Reconciliation is idempotent; retries keep the same object reference.
	if err = uploadAsset(ctx, db, s, id, "audio", filename, "audio/mpeg", ".mp3"); err != nil {
		t.Fatal(err)
	}
	if err = exportTranscript(ctx, db, s, id); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, "SELECT object_key FROM storage_exports WHERE recording_id=$1", id).Scan(&key); err != nil {
		t.Fatal(err)
	}
	keys = append(keys, key)
	obj, err := s.Client.GetObject(ctx, s.Bucket, key, minio.GetObjectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(obj)
	obj.Close()
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err = json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["title"] != "storage test" {
		t.Fatal("incorrect transcript snapshot")
	}
	// Default IPA changes without a text revision still produce a fresh snapshot.
	if _, err = db.Exec(ctx, "UPDATE recordings SET default_ipa_voice='en-us' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	if err = exportTranscript(ctx, db, s, id); err != nil {
		t.Fatal(err)
	}
	var next string
	if err = db.QueryRow(ctx, "SELECT object_key FROM storage_exports WHERE recording_id=$1", id).Scan(&next); err != nil {
		t.Fatal(err)
	}
	keys = append(keys, next)
	if next == key {
		t.Fatal("IPA change failed to invalidate JSON snapshot")
	}
}
