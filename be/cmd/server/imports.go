package main

import (
	"context"
	"crypto/rand"
	"dharma-player/internal/core"
	"dharma-player/internal/objectstore"
	"dharma-player/internal/phonetics"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func jsonResponse(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func registerImportRoutes(mux *http.ServeMux, db *pgxpool.Pool, dir string, stores ...*objectstore.Store) {
	var store *objectstore.Store
	if len(stores) > 0 {
		store = stores[0]
	}
	registerYouTubeRoutes(mux, db)
	registerPDFRoutes(mux, db, dir, store)
	mux.HandleFunc("POST /api/recordings", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 200<<20)
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			http.Error(w, "MP3 tối đa 200 MB; dữ liệu tải lên không hợp lệ", 400)
			return
		}
		defer r.MultipartForm.RemoveAll()
		title := strings.TrimSpace(r.FormValue("title"))
		if title == "" || len([]rune(title)) > 200 {
			http.Error(w, "Tên bài cần từ 1 đến 200 ký tự", 400)
			return
		}
		src, header, err := r.FormFile("audio")
		if err != nil {
			http.Error(w, "Chọn file MP3", 400)
			return
		}
		defer src.Close()
		if strings.ToLower(filepath.Ext(header.Filename)) != ".mp3" {
			http.Error(w, "Chỉ nhận file MP3", 400)
			return
		}
		id := newID()
		name := id + ".mp3"
		path := filepath.Join(dir, name)
		dst, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0640)
		if err != nil {
			http.Error(w, "Không lưu được audio", 500)
			return
		}
		n, copyErr := io.Copy(dst, src)
		closeErr := dst.Close()
		keep := false
		defer func() {
			if !keep {
				os.Remove(path)
			}
		}()
		if copyErr != nil || closeErr != nil || n == 0 {
			http.Error(w, "Không đọc được file MP3", 400)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-select_streams", "a:0", "-show_entries", "stream=codec_name:format=duration", "-of", "json", path)
		output, err := cmd.Output()
		var probe struct {
			Streams []struct {
				Codec string `json:"codec_name"`
			} `json:"streams"`
			Format struct {
				Duration string `json:"duration"`
			} `json:"format"`
		}
		if err != nil || json.Unmarshal(output, &probe) != nil || len(probe.Streams) == 0 || probe.Streams[0].Codec != "mp3" {
			http.Error(w, "File không chứa audio MP3 hợp lệ", 400)
			return
		}
		var seconds float64
		fmt.Sscan(probe.Format.Duration, &seconds)
		if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 || seconds > 4*3600 {
			http.Error(w, "Audio phải có thời lượng từ trên 0 đến 4 giờ", 400)
			return
		}
		var objectKey, hash string
		var objectSize int64
		if store != nil {
			hash, objectSize, err = objectstore.HashFile(path)
			if err == nil {
				objectKey = store.Key(id, "audio", hash, ".mp3")
				err = store.Upload(r.Context(), objectKey, path, "audio/mpeg", hash)
			}
			if err != nil {
				log.Print(err)
				http.Error(w, "Không lưu được audio lên kho lưu trữ; hãy thử lại", 503)
				return
			}
		}
		tx, err := db.Begin(r.Context())
		if err != nil {
			http.Error(w, "Không lưu được bài", 500)
			return
		}
		defer tx.Rollback(r.Context())
		jobID := newID()
		if _, err = tx.Exec(r.Context(), "INSERT INTO sutras VALUES($1,$2,'en')", id, title); err == nil {
			_, err = tx.Exec(r.Context(), "INSERT INTO recordings(id,sutra_id,title,audio_file,source_url,status,duration_ms) VALUES($1,$1,$2,$3,'','processing',$4)", id, title, name, int64(math.Round(seconds*1000)))
		}
		if err == nil && store != nil {
			_, err = tx.Exec(r.Context(), "INSERT INTO recording_assets(recording_id,kind,object_key,content_type,size_bytes,sha256) VALUES($1,'audio',$2,'audio/mpeg',$3,$4)", id, objectKey, objectSize, hash)
		}
		if err == nil {
			_, err = tx.Exec(r.Context(), "INSERT INTO processing_jobs(id,recording_id,kind,status) VALUES($1,$2,'transcribe','queued')", jobID, id)
		}
		if err == nil {
			err = tx.Commit(r.Context())
		}
		if err != nil {
			log.Print(err)
			http.Error(w, "Không lưu được bài", 500)
			return
		}
		keep = true
		jsonResponse(w, 202, map[string]string{"recordingId": id, "jobId": jobID})
	})
	mux.HandleFunc("GET /api/recordings/{id}/job", func(w http.ResponseWriter, r *http.Request) {
		var id, kind, status, message, progressMessage, voice string
		var progress int
		err := db.QueryRow(r.Context(), "SELECT id,kind,status,error,progress,progress_message,COALESCE(payload->>'voice','') FROM processing_jobs WHERE recording_id=$1 ORDER BY created_at DESC LIMIT 1", r.PathValue("id")).Scan(&id, &kind, &status, &message, &progress, &progressMessage, &voice)
		if err == pgx.ErrNoRows {
			jsonResponse(w, 200, map[string]any{"status": "ready", "progress": 100})
			return
		}
		if err != nil {
			http.Error(w, "Không tải được trạng thái", 500)
			return
		}
		jsonResponse(w, 200, map[string]any{"id": id, "kind": kind, "status": status, "error": message, "progress": progress, "progressMessage": progressMessage, "voice": voice})
	})
	mux.HandleFunc("POST /api/recordings/{id}/retry", func(w http.ResponseWriter, r *http.Request) {
		tag, err := db.Exec(r.Context(), `UPDATE processing_jobs SET status=CASE WHEN kind='ipa' THEN 'aligned' ELSE 'queued' END,progress=0,progress_message='',error='',result=NULL,updated_at=now() WHERE id=(SELECT id FROM processing_jobs WHERE recording_id=$1 ORDER BY created_at DESC LIMIT 1) AND status='failed'`, r.PathValue("id"))
		if err != nil || tag.RowsAffected() != 1 {
			http.Error(w, "Không có tác vụ lỗi để thử lại", 409)
			return
		}
		jsonResponse(w, 202, map[string]string{"status": "queued"})
	})
	mux.HandleFunc("PATCH /api/recordings/{id}/segments/{segment}", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
		var input struct {
			Text string `json:"text"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Text) == "" || len([]rune(input.Text)) > 4000 {
			http.Error(w, "Đoạn lời cần từ 1 đến 4000 ký tự", 400)
			return
		}
		id := r.PathValue("id")
		segmentID := r.PathValue("segment")
		// Original sample lives on a different readonly mount; the worker knows that path.
		var file string
		var revision int
		var start, end int
		err := db.QueryRow(r.Context(), `SELECT r.audio_file,r.revision,s.start_ms,s.end_ms FROM recordings r JOIN segments s ON s.recording_id=r.id WHERE r.id=$1 AND s.id=$2`, id, segmentID).Scan(&file, &revision, &start, &end)
		if err == pgx.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "Không đọc được đoạn lời", 500)
			return
		}
		payload, _ := json.Marshal(map[string]any{"segmentId": segmentID, "text": strings.TrimSpace(input.Text), "startMs": start, "endMs": end})
		jobID := newID()
		_, err = db.Exec(r.Context(), "INSERT INTO processing_jobs(id,recording_id,kind,status,payload,base_revision) VALUES($1,$2,'edit','queued',$3,$4)", jobID, id, payload, revision)
		if err != nil {
			http.Error(w, "Bài đang có tác vụ khác; hãy chờ xử lý xong", 409)
			return
		}
		jsonResponse(w, 202, map[string]string{"jobId": jobID, "status": "queued"})
	})
}

// PostgreSQL is the durable queue. Worker alignment completes before API creates IPA.
func processAligned(ctx context.Context, db *pgxpool.Pool, g phonetics.Generator) {
	if _, err := db.Exec(ctx, "UPDATE processing_jobs SET status='aligned',progress=90,progress_message='Chuẩn bị tạo lại phiên âm' WHERE status='phonetics'"); err != nil {
		log.Print(err)
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		func() {
			tx, err := db.Begin(ctx)
			if err != nil {
				return
			}
			defer tx.Rollback(ctx)
			var jobID, id, kind string
			var result, payload []byte
			var revision int
			err = tx.QueryRow(ctx, `SELECT id,recording_id,kind,result,payload,base_revision FROM processing_jobs WHERE status='aligned' ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&jobID, &id, &kind, &result, &payload, &revision)
			if err == pgx.ErrNoRows {
				return
			}
			if err != nil {
				log.Print(err)
				return
			}
			_, err = tx.Exec(ctx, "UPDATE processing_jobs SET status='phonetics',progress=CASE WHEN kind='ipa' THEN 0 ELSE 90 END,progress_message='Đang tạo phiên âm từng từ',updated_at=now() WHERE id=$1", jobID)
			if err != nil {
				return
			}
			if err = tx.Commit(ctx); err != nil {
				return
			}
			if err = finishJob(ctx, db, g, id, jobID, kind, result, payload, revision); err != nil {
				log.Printf("job %s: %v", jobID, err)
				if ctx.Err() == nil {
					db.Exec(ctx, "UPDATE processing_jobs SET status='failed',error=$2,updated_at=now() WHERE id=$1", jobID, "Không hoàn tất dữ liệu: "+err.Error())
				}
			}
		}()
	}
}

func finishJob(ctx context.Context, db *pgxpool.Pool, g phonetics.Generator, id, jobID, kind string, result, payload []byte, revision int) error {
	t, err := readTranscript(ctx, db, id)
	if err != nil {
		return err
	}
	if kind == "ipa" {
		return finishIPAJob(ctx, db, g, id, jobID, payload, revision, t)
	}
	// Import creates only British IPA; edits use this recording's chosen default.
	g.Voice = "en-gb"
	if kind == "edit" && t.DefaultIPAVoice != "" {
		g.Voice = t.DefaultIPAVoice
	}
	var segments []core.Segment
	if err = json.Unmarshal(result, &segments); err != nil {
		return err
	}
	candidate := core.Transcript{Version: 1, Title: t.Title, Language: t.Language, Segments: segments}
	if err = core.Normalize(&candidate); err != nil {
		return err
	}
	var edited struct {
		SegmentID string `json:"segmentId"`
	}
	if kind == "edit" {
		if err = json.Unmarshal(payload, &edited); err != nil {
			return err
		}
		if len(candidate.Segments) != 1 {
			return fmt.Errorf("expected one edited segment")
		}
		for _, old := range t.Segments {
			if old.ID == edited.SegmentID {
				preserveIPA(old.Tokens, candidate.Segments[0].Tokens)
				for i := range candidate.Segments[0].Tokens {
					candidate.Segments[0].Tokens[i].Phonetic = candidate.Segments[0].Tokens[i].Phonetics[g.Voice]
				}
			}
		}
	}
	lastProgress := -1
	g.OnProgress = func(done, total int) error {
		percent := 98
		if total > 0 {
			percent = 90 + 8*done/total
		}
		if percent == lastProgress {
			return nil
		}
		_, err := db.Exec(ctx, "UPDATE processing_jobs SET progress=$2,progress_message=$3,updated_at=now() WHERE id=$1 AND status='phonetics'", jobID, percent, fmt.Sprintf("Đã tạo IPA cho %d/%d từ", done, total))
		if err == nil {
			lastProgress = percent
		}
		return err
	}
	if _, err = g.Enrich(ctx, &candidate); err != nil {
		return err
	}
	if _, err = db.Exec(ctx, "UPDATE processing_jobs SET progress=99,progress_message='Đang lưu lời và phiên âm',updated_at=now() WHERE id=$1 AND status='phonetics'", jobID); err != nil {
		return err
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var actual int
	if err = tx.QueryRow(ctx, "SELECT revision FROM recordings WHERE id=$1 FOR UPDATE", id).Scan(&actual); err != nil {
		return err
	}
	if actual != revision {
		return fmt.Errorf("lời đã thay đổi; hãy tải lại và sửa lại")
	}
	if kind == "transcribe" {
		if _, err = tx.Exec(ctx, "DELETE FROM segments WHERE recording_id=$1", id); err != nil {
			return err
		}
	}
	for i, segment := range candidate.Segments {
		segmentID := id + "-" + segment.ID
		if kind == "edit" {
			segmentID = edited.SegmentID
			var oldStart, oldEnd int
			if err = tx.QueryRow(ctx, "SELECT start_ms,end_ms FROM segments WHERE id=$1 AND recording_id=$2", segmentID, id).Scan(&oldStart, &oldEnd); err != nil {
				return err
			}
			if segment.StartMs != oldStart || segment.EndMs != oldEnd {
				return fmt.Errorf("edited segment boundary changed")
			}
			if _, err = tx.Exec(ctx, "DELETE FROM tokens WHERE segment_id=$1", segmentID); err != nil {
				return err
			}
			_, err = tx.Exec(ctx, "UPDATE segments SET text=$1 WHERE id=$2", segment.Text, segmentID)
		} else {
			_, err = tx.Exec(ctx, "INSERT INTO segments VALUES($1,$2,$3,$4,$5,$6)", segmentID, id, i, segment.StartMs, segment.EndMs, segment.Text)
		}
		if err != nil {
			return err
		}
		for j, token := range segment.Tokens {
			var ipa []byte
			if token.Phonetic != nil {
				ipa, _ = json.Marshal(token.Phonetic)
			}
			tokenID := fmt.Sprintf("%s-token-%04d", segmentID, j+1)
			if _, err = tx.Exec(ctx, "INSERT INTO tokens VALUES($1,$2,$3,$4,$5,$6,$7,$8)", tokenID, segmentID, j, token.Text, token.StartMs, token.EndMs, token.EndEstimated, ipa); err != nil {
				return err
			}
			for voice, value := range token.Phonetics {
				if value != nil {
					if err = saveTokenIPA(ctx, tx, tokenID, voice, value); err != nil {
						return err
					}
				}
			}
			if token.Phonetic != nil {
				if err = saveTokenIPA(ctx, tx, tokenID, g.Voice, token.Phonetic); err != nil {
					return err
				}
			}
		}
	}
	if kind == "transcribe" {
		if _, err = tx.Exec(ctx, "DELETE FROM recording_ipa_versions WHERE recording_id=$1", id); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, "INSERT INTO recording_ipa_versions(recording_id,voice,revision) VALUES($1,$2,$3) ON CONFLICT(recording_id,voice) DO UPDATE SET revision=EXCLUDED.revision,generated_at=now()", id, g.Voice, revision+1); err != nil {
		return err
	}
	// Unchanged words keep both accents, but other versions require completion after a text edit.
	if _, err = tx.Exec(ctx, "UPDATE recordings SET status='ready',revision=revision+1,default_ipa_voice=$2 WHERE id=$1", id, g.Voice); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "UPDATE processing_jobs SET status='ready',progress=100,progress_message='Hoàn tất',error='',updated_at=now() WHERE id=$1", jobID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Preserve reviewed pronunciation for unchanged words at the same position.
func preserveIPA(old, updated []core.Token) {
	for i := range updated {
		if i < len(old) && updated[i].Text == old[i].Text {
			updated[i].Phonetic = old[i].Phonetic
			updated[i].Phonetics = old[i].Phonetics
		}
	}
}
