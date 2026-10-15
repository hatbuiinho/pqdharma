package main

import (
	"context"
	"dharma-player/internal/core"
	"dharma-player/internal/objectstore"
	"dharma-player/internal/phonetics"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	sample := flag.String("sample", "../imports/aaaS89avDVU/import.json", "sample JSON")
	addr := flag.String("addr", ":8080", "listen address")
	fileMode := flag.Bool("file-mode", false, "explicit demo mode without PostgreSQL")
	autoIPA := flag.Bool("auto-ipa", true, "generate missing English IPA using backend espeak-ng")
	ipaVoice := flag.String("ipa-voice", "en-gb", "IPA voice: en-us or en-gb")
	ipaOverrides := flag.String("ipa-overrides", "", "optional JSON map of terms to reviewed IPA")
	syncOnce := flag.Bool("sync-storage-once", false, "migrate local media and export transcripts to S3, retaining local files")
	flag.Parse()
	store, storageErr := objectstore.FromEnv()
	if storageErr != nil {
		log.Fatal(storageErr)
	}
	if *syncOnce && (store == nil || *fileMode) {
		log.Fatal("-sync-storage-once requires S3 configuration and PostgreSQL")
	}
	t, e := core.Load(*sample)
	if e != nil {
		if *fileMode || !os.IsNotExist(e) {
			log.Fatal(e)
		}
		log.Printf("Sample unavailable; starting with database recordings only: %v", e)
	}
	sampleAvailable := e == nil
	sampleExists := false
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var db *pgxpool.Pool
	if !*fileMode {
		url := os.Getenv("DATABASE_URL")
		if url == "" {
			log.Fatal("DATABASE_URL required; use -file-mode for local preview")
		}
		db, e = pgxpool.New(ctx, url)
		if e != nil {
			log.Fatal(e)
		}
		defer db.Close()
		if e = db.Ping(ctx); e != nil {
			log.Fatal(e)
		}
		sql, e := os.ReadFile("migrations/001_init.sql")
		if e != nil {
			log.Fatal(e)
		}
		if _, e = db.Exec(ctx, string(sql)); e != nil {
			log.Fatal(e)
		}
		for _, path := range []string{"migrations/002_jobs.sql", "migrations/003_job_progress.sql", "migrations/004_ipa_voices.sql", "migrations/005_media.sql", "migrations/006_duration.sql"} {
			migration, err := os.ReadFile(path)
			if err != nil {
				log.Fatal(err)
			}
			if _, err = db.Exec(ctx, string(migration)); err != nil {
				log.Fatal(err)
			}
		}
		// Seed exactly once. Existing database edits survive restarts.
		var exists bool
		e = db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM recordings WHERE id='sample')").Scan(&exists)
		if e != nil {
			log.Fatal(e)
		}
		sampleExists = exists
		if !exists && sampleAvailable {
			tx, e := db.Begin(ctx)
			if e != nil {
				log.Fatal(e)
			}
			defer tx.Rollback(ctx)
			_, e = tx.Exec(ctx, "INSERT INTO sutras VALUES('sample',$1,$2);", t.Title, t.Language)
			if e != nil {
				log.Fatal(e)
			}
			_, e = tx.Exec(ctx, "INSERT INTO recordings(id,sutra_id,title,audio_file,source_url,status) VALUES('sample','sample',$1,$2,$3,$4)", t.Recording.Title, t.Recording.AudioFile, t.SourceURL, t.Status)
			if e != nil {
				log.Fatal(e)
			}
			for i, s := range t.Segments {
				_, e = tx.Exec(ctx, "INSERT INTO segments VALUES($1,'sample',$2,$3,$4,$5)", s.ID, i, s.StartMs, s.EndMs, s.Text)
				if e != nil {
					log.Fatal(e)
				}
				for j, w := range s.Tokens {
					var phonetic []byte
					if w.Phonetic != nil {
						phonetic, _ = json.Marshal(w.Phonetic)
					}
					_, e = tx.Exec(ctx, "INSERT INTO tokens VALUES($1,$2,$3,$4,$5,$6,$7,$8)", w.ID, s.ID, j, w.Text, w.StartMs, w.EndMs, w.EndEstimated, phonetic)
					if e != nil {
						log.Fatal(e)
					}
				}
			}
			if e = tx.Commit(ctx); e != nil {
				log.Fatal(e)
			}
			sampleExists = true
		}
	}
	if *autoIPA && (*fileMode || sampleExists) {
		overrides, err := phonetics.LoadOverrides(*ipaOverrides)
		if err != nil {
			log.Fatal(err)
		}
		target := &t
		if db != nil {
			existing, err := readTranscript(ctx, db, "sample")
			if err != nil {
				log.Fatal(err)
			}
			target = &existing
		}
		if db != nil && target.DefaultIPAVoice == "" { // Existing legacy sample: preserve it until the user requests a named version.
			target = nil
		}
		count := 0
		if target != nil {
			voice := *ipaVoice
			if db != nil {
				voice = target.DefaultIPAVoice
			}
			count, err = (phonetics.Generator{Voice: voice, Overrides: overrides}).Enrich(ctx, target)
		}
		if err != nil {
			log.Fatal("Generate IPA: ", err)
		}
		if db != nil && target != nil && count > 0 {
			if err := saveMissingIPA(ctx, db, *target); err != nil {
				log.Fatal(err)
			}
		}
		log.Printf("IPA (%s): generated %d tokens", *ipaVoice, count)
	}

	send := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(v)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		if db != nil {
			c, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			if e := db.Ping(c); e != nil {
				http.Error(w, "Database unavailable", 503)
				return
			}
		}
		send(w, map[string]any{"ok": true, "fileMode": *fileMode})
	})
	uploadDir := os.Getenv("UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = "../uploads"
	}
	overrides, err := phonetics.LoadOverrides(*ipaOverrides)
	if err != nil {
		log.Fatal(err)
	}
	if db != nil {
		if err := os.MkdirAll(uploadDir, 0750); err != nil {
			log.Fatal(err)
		}
		if store != nil {
			if *syncOnce {
				if err := syncStorage(ctx, db, store, uploadDir, *sample); err != nil {
					log.Fatal(err)
				}
				log.Print("Storage synchronization completed; local files retained")
				return
			}
			registerStoredMedia(mux, db, store)
			go runStorageSync(ctx, db, store, uploadDir, *sample)
		}
		go processAligned(ctx, db, phonetics.Generator{Voice: *ipaVoice, Overrides: overrides})
		registerImportRoutes(mux, db, uploadDir, store)
		registerIPARoutes(mux, db)
	}
	mux.HandleFunc("GET /api/sutras", func(w http.ResponseWriter, r *http.Request) {
		if db == nil {
			send(w, []map[string]any{{"id": "sample", "title": t.Title, "language": t.Language, "recordingId": "sample", "status": "ready"}})
			return
		}
		rows, err := db.Query(r.Context(), `SELECT r.id,s.title,s.language,COALESCE(j.status,r.status),COALESCE(j.progress,100),COALESCE(a.sha256,''),COALESCE(r.duration_ms,0) FROM recordings r JOIN sutras s ON s.id=r.sutra_id LEFT JOIN recording_assets a ON a.recording_id=r.id AND a.kind='thumbnails' LEFT JOIN LATERAL (SELECT status,progress FROM processing_jobs WHERE recording_id=r.id ORDER BY created_at DESC LIMIT 1) j ON true ORDER BY r.id='sample' DESC,r.id`)
		if err != nil {
			http.Error(w, "Không tải được thư viện", 500)
			return
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var id, title, language, status, thumbnailHash string
			var progress int
			var durationMs int64
			if err := rows.Scan(&id, &title, &language, &status, &progress, &thumbnailHash, &durationMs); err != nil {
				http.Error(w, "Không tải được thư viện", 500)
				return
			}
			if status == "draft" {
				status = "ready"
			}
			thumbnailURL := ""
			if store != nil && thumbnailHash != "" {
				thumbnailURL = "/api/recordings/" + id + "/thumbnail?v=" + thumbnailHash
			}
			out = append(out, map[string]any{"durationMs": durationMs, "thumbnailUrl": thumbnailURL, "id": id, "title": title, "language": language, "recordingId": id, "status": status, "progress": progress})
		}
		if rows.Err() != nil {
			http.Error(w, "Không tải được thư viện", 500)
			return
		}
		send(w, out)
	})
	mux.HandleFunc("GET /api/recordings/{id}/transcript", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if db == nil {
			if id != "sample" {
				http.NotFound(w, r)
				return
			}
			send(w, phonetics.PrepareReadable(t))
			return
		}
		out, err := readTranscript(r.Context(), db, id)
		if err != nil {
			if err == pgx.ErrNoRows {
				http.NotFound(w, r)
			} else {
				log.Print(err)
				http.Error(w, "Không tải được lời", 500)
			}
			return
		}
		if len(out.Segments) == 0 {
			http.Error(w, "Bài đang được xử lý", 409)
			return
		}
		send(w, phonetics.PrepareReadable(out))
	})
	mux.HandleFunc("GET /api/recordings/{id}/audio", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if db != nil && store != nil {
			var key string
			err := db.QueryRow(r.Context(), "SELECT object_key FROM recording_assets WHERE recording_id=$1 AND kind='audio'", id).Scan(&key)
			if err == nil {
				store.Serve(w, r, key, "audio/mpeg")
				return
			}
			if err != pgx.ErrNoRows {
				http.Error(w, "Storage unavailable", 503)
				return
			}
		}
		audioPath := filepath.Join(filepath.Dir(*sample), t.Recording.AudioFile)
		if db != nil || id != "sample" {
			if db == nil {
				http.NotFound(w, r)
				return
			}
			var file string
			if err := db.QueryRow(r.Context(), "SELECT audio_file FROM recordings WHERE id=$1", id).Scan(&file); err != nil {
				http.NotFound(w, r)
				return
			}
			if id == "sample" {
				audioPath = filepath.Join(filepath.Dir(*sample), filepath.Base(file))
			} else {
				audioPath = filepath.Join(uploadDir, filepath.Base(file))
			}
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		http.ServeFile(w, r, audioPath)
	})
	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(c)
	}()
	fmt.Printf("Dharma API listening on %s (file mode: %v)\n", *addr, *fileMode)
	if e = srv.ListenAndServe(); e != nil && e != http.ErrServerClosed {
		log.Fatal(e)
	}
}

type transcriptReader interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func readTranscript(ctx context.Context, db transcriptReader, recordingID string) (core.Transcript, error) {
	var t core.Transcript
	t.Version = 1
	e := db.QueryRow(ctx, "SELECT s.title,s.language,r.title,r.audio_file,r.source_url,r.status,r.revision,COALESCE(r.default_ipa_voice,'') FROM recordings r JOIN sutras s ON s.id=r.sutra_id WHERE r.id=$1", recordingID).Scan(&t.Title, &t.Language, &t.Recording.Title, &t.Recording.AudioFile, &t.SourceURL, &t.Status, &t.Revision, &t.DefaultIPAVoice)
	if e != nil {
		return t, e
	}
	rows, e := db.Query(ctx, "SELECT id,start_ms,end_ms,text FROM segments WHERE recording_id=$1 ORDER BY position", recordingID)
	if e != nil {
		return t, e
	}
	defer rows.Close()
	for rows.Next() {
		var s core.Segment
		if e = rows.Scan(&s.ID, &s.StartMs, &s.EndMs, &s.Text); e != nil {
			return t, e
		}
		t.Segments = append(t.Segments, s)
	}
	if e = rows.Err(); e != nil {
		return t, e
	}
	rows.Close()
	positions := make(map[string]int, len(t.Segments))
	for i, s := range t.Segments {
		positions[s.ID] = i
	}
	rs, e := db.Query(ctx, "SELECT w.id,w.segment_id,w.text,w.start_ms,w.end_ms,w.end_estimated,w.phonetic,COALESCE((SELECT jsonb_object_agg(v.voice,v.phonetic) FROM token_ipa v WHERE v.token_id=w.id),'{}'::jsonb) FROM tokens w JOIN segments s ON s.id=w.segment_id WHERE s.recording_id=$1 ORDER BY s.position,w.position", recordingID)
	if e != nil {
		return t, e
	}
	defer rs.Close()
	for rs.Next() {
		var w core.Token
		var segmentID string
		var p, variants []byte
		if e = rs.Scan(&w.ID, &segmentID, &w.Text, &w.StartMs, &w.EndMs, &w.EndEstimated, &p, &variants); e != nil {
			return t, e
		}
		if len(p) > 0 {
			if e = json.Unmarshal(p, &w.Phonetic); e != nil {
				return t, e
			}
		}
		if e = json.Unmarshal(variants, &w.Phonetics); e != nil {
			return t, e
		}
		if t.DefaultIPAVoice != "" {
			w.Phonetic = w.Phonetics[t.DefaultIPAVoice]
		}
		index, ok := positions[segmentID]
		if !ok {
			return t, fmt.Errorf("unknown segment")
		}
		t.Segments[index].Tokens = append(t.Segments[index].Tokens, w)
	}
	if e = rs.Err(); e != nil {
		return t, e
	}

	rs.Close()
	t.IPAEditable = true
	t.IPAVoices = []string{}
	versions, err := db.Query(ctx, "SELECT voice FROM recording_ipa_versions WHERE recording_id=$1 AND revision=$2 ORDER BY voice", recordingID, t.Revision)
	if err != nil {
		return t, err
	}
	defer versions.Close()
	for versions.Next() {
		var voice string
		if err = versions.Scan(&voice); err != nil {
			return t, err
		}
		t.IPAVoices = append(t.IPAVoices, voice)
	}
	return t, versions.Err()
}

// Fill gaps only; concurrent edits and existing reviewed pronunciations survive.
func saveMissingIPA(ctx context.Context, db *pgxpool.Pool, t core.Transcript) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, segment := range t.Segments {
		for _, token := range segment.Tokens {
			if token.Phonetic == nil {
				continue
			}
			value, err := json.Marshal(token.Phonetic)
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `UPDATE tokens SET phonetic=$1 WHERE id=$2 AND text=$3 AND phonetic IS NULL AND segment_id IN (SELECT id FROM segments WHERE recording_id='sample')`, value, token.ID, token.Text)
			if err != nil {
				return err
			}
		}
	}
	if t.DefaultIPAVoice != "" {
		if err = saveTranscriptIPA(ctx, tx, "sample", t.DefaultIPAVoice, t.Revision, t); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
