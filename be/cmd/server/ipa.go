package main

import (
	"context"
	"dharma-player/internal/core"
	"dharma-player/internal/phonetics"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func validIPAVoice(voice string) bool { return voice == "en-gb" || voice == "en-us" }

func registerIPARoutes(mux *http.ServeMux, db *pgxpool.Pool) {
	mux.HandleFunc("POST /api/recordings/{id}/ipa", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		var input struct {
			Voice string `json:"voice"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || !validIPAVoice(input.Voice) {
			http.Error(w, "Chọn IPA Anh–Anh hoặc Anh–Mỹ", 400)
			return
		}
		tx, err := db.Begin(r.Context())
		if err != nil {
			http.Error(w, "Không tạo được tác vụ IPA", 500)
			return
		}
		defer tx.Rollback(r.Context())
		id := r.PathValue("id")
		var revision, count int
		var language string
		err = tx.QueryRow(r.Context(), "SELECT r.revision,s.language FROM recordings r JOIN sutras s ON s.id=r.sutra_id WHERE r.id=$1 FOR UPDATE OF r", id).Scan(&revision, &language)
		if err == pgx.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "Không đọc được bài", 500)
			return
		}
		if language != "en" && language != "en-gb" && language != "en-us" {
			http.Error(w, "IPA chỉ hỗ trợ bài tiếng Anh", 400)
			return
		}
		if err = tx.QueryRow(r.Context(), "SELECT count(*) FROM segments WHERE recording_id=$1", id).Scan(&count); err != nil {
			http.Error(w, "Không đọc được lời", 500)
			return
		}
		if count == 0 {
			http.Error(w, "Chờ xử lý lời kinh trước khi tạo IPA", 409)
			return
		}
		payload, _ := json.Marshal(input)
		jobID := newID()
		// IPA enters the API queue directly; no transcription or audio alignment runs.
		_, err = tx.Exec(r.Context(), "INSERT INTO processing_jobs(id,recording_id,kind,status,payload,base_revision,progress_message) VALUES($1,$2,'ipa','aligned',$3,$4,'Chuẩn bị tạo IPA')", jobID, id, payload, revision)
		if err != nil {
			http.Error(w, "Bài đang có tác vụ khác; hãy chờ xử lý xong", 409)
			return
		}
		if err = tx.Commit(r.Context()); err != nil {
			http.Error(w, "Không tạo được tác vụ IPA", 500)
			return
		}
		jsonResponse(w, 202, map[string]string{"jobId": jobID, "status": "aligned", "voice": input.Voice})
	})
	mux.HandleFunc("PATCH /api/recordings/{id}/ipa/default", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		var input struct {
			Voice string `json:"voice"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || !validIPAVoice(input.Voice) {
			http.Error(w, "Chọn IPA Anh–Anh hoặc Anh–Mỹ", 400)
			return
		}
		tx, err := db.Begin(r.Context())
		if err != nil {
			http.Error(w, "Không lưu được giọng mặc định", 500)
			return
		}
		defer tx.Rollback(r.Context())
		id := r.PathValue("id")
		var revision int
		err = tx.QueryRow(r.Context(), "SELECT revision FROM recordings WHERE id=$1 FOR UPDATE", id).Scan(&revision)
		if err == pgx.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "Không đọc được bài", 500)
			return
		}
		var busy bool
		if err = tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM processing_jobs WHERE recording_id=$1 AND status NOT IN ('ready','failed'))", id).Scan(&busy); err != nil {
			http.Error(w, "Không đọc được trạng thái bài", 500)
			return
		}
		if busy {
			http.Error(w, "Chờ tác vụ hiện tại hoàn tất trước khi đổi mặc định", 409)
			return
		}
		var ready bool
		err = tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM recording_ipa_versions WHERE recording_id=$1 AND voice=$2 AND revision=$3)", id, input.Voice, revision).Scan(&ready)
		if err != nil {
			http.Error(w, "Không đọc được IPA", 500)
			return
		}
		if !ready {
			http.Error(w, "Tạo xong bản IPA này trước khi đặt mặc định", 409)
			return
		}
		if _, err = tx.Exec(r.Context(), "UPDATE recordings SET default_ipa_voice=$2 WHERE id=$1", id, input.Voice); err != nil {
			http.Error(w, "Không lưu được giọng mặc định", 500)
			return
		}
		if err = tx.Commit(r.Context()); err != nil {
			http.Error(w, "Không lưu được giọng mặc định", 500)
			return
		}
		jsonResponse(w, 200, map[string]string{"defaultIpaVoice": input.Voice})
	})
}

func saveTokenIPA(ctx context.Context, tx pgx.Tx, id, voice string, value *core.Phonetic) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "INSERT INTO token_ipa(token_id,voice,phonetic) VALUES($1,$2,$3) ON CONFLICT(token_id,voice) DO UPDATE SET phonetic=EXCLUDED.phonetic", id, voice, data)
	return err
}

func saveTranscriptIPA(ctx context.Context, tx pgx.Tx, id, voice string, revision int, t core.Transcript) error {
	for _, segment := range t.Segments {
		for _, token := range segment.Tokens {
			if token.Phonetic != nil {
				if err := saveTokenIPA(ctx, tx, token.ID, voice, token.Phonetic); err != nil {
					return err
				}
			}
		}
	}
	_, err := tx.Exec(ctx, "INSERT INTO recording_ipa_versions(recording_id,voice,revision) VALUES($1,$2,$3) ON CONFLICT(recording_id,voice) DO UPDATE SET revision=EXCLUDED.revision,generated_at=now()", id, voice, revision)
	return err
}

func finishIPAJob(ctx context.Context, db *pgxpool.Pool, g phonetics.Generator, id, jobID string, payload []byte, revision int, t core.Transcript) error {
	var input struct {
		Voice string `json:"voice"`
	}
	if err := json.Unmarshal(payload, &input); err != nil {
		return err
	}
	if !validIPAVoice(input.Voice) {
		return fmt.Errorf("unsupported IPA voice")
	}
	if len(t.Segments) == 0 || t.Revision != revision {
		return fmt.Errorf("lời đã thay đổi; hãy tạo lại IPA")
	}
	g.Voice = input.Voice
	for si := range t.Segments {
		for ti := range t.Segments[si].Tokens {
			token := &t.Segments[si].Tokens[ti]
			// Regenerate automatic IPA only. Unannotated or manual corrections are preserved per accent.
			token.Phonetic = token.Phonetics[input.Voice]
			if token.Phonetic != nil && (token.Phonetic.Source == "generated" || token.Phonetic.Source == "override") {
				token.Phonetic = nil
			}
		}
	}
	lastProgress := -1
	g.OnProgress = func(done, total int) error {
		percent := 98
		if total > 0 {
			percent = 98 * done / total
		}
		if percent == lastProgress {
			return nil
		}
		_, err := db.Exec(ctx, "UPDATE processing_jobs SET progress=$2,progress_message=$3,updated_at=now() WHERE id=$1 AND status='phonetics'", jobID, percent, fmt.Sprintf("IPA %s · %d/%d từ", map[string]string{"en-gb": "Anh–Anh", "en-us": "Anh–Mỹ"}[input.Voice], done, total))
		if err == nil {
			lastProgress = percent
		}
		return err
	}
	if _, err := g.Enrich(ctx, &t); err != nil {
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
		return fmt.Errorf("lời đã thay đổi; hãy tạo lại IPA")
	}
	if err = saveTranscriptIPA(ctx, tx, id, input.Voice, revision, t); err != nil {
		return err
	}
	// Legacy recordings gain an explicit default on their first named IPA generation.
	if _, err = tx.Exec(ctx, "UPDATE recordings SET default_ipa_voice=$2 WHERE id=$1 AND default_ipa_voice IS NULL", id, input.Voice); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "UPDATE processing_jobs SET status='ready',progress=100,progress_message='Hoàn tất IPA',error='',updated_at=now() WHERE id=$1", jobID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
