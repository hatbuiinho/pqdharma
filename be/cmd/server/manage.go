package main

import (
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"strings"
)

func registerManagement(mux *http.ServeMux, db *pgxpool.Pool) {
	registerRecordingAccess(mux, db)
	mux.HandleFunc("PATCH /api/recordings/{id}", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 16384)
		var input struct {
			Title      string   `json:"title"`
			Visibility string   `json:"visibility"`
			Emails     []string `json:"emails"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Title) == "" || len([]rune(input.Title)) > 200 || len(input.Emails) > 100 || (input.Visibility != "" && input.Visibility != "private" && input.Visibility != "shared" && input.Visibility != "public") {
			http.Error(w, "Tên hoặc quyền truy cập không hợp lệ", 400)
			return
		}
		tx, err := db.Begin(r.Context())
		if err != nil {
			http.Error(w, "Không lưu được", 500)
			return
		}
		defer tx.Rollback(r.Context())
		_, err = tx.Exec(r.Context(), "UPDATE recordings SET title=$2,visibility=CASE WHEN $3='' THEN visibility ELSE $3 END WHERE id=$1", r.PathValue("id"), strings.TrimSpace(input.Title), input.Visibility)
		if err == nil {
			_, err = tx.Exec(r.Context(), "UPDATE sutras SET title=$2 WHERE id=(SELECT sutra_id FROM recordings WHERE id=$1)", r.PathValue("id"), strings.TrimSpace(input.Title))
		}
		if err == nil && input.Visibility != "" {
			_, err = tx.Exec(r.Context(), "DELETE FROM recording_shares WHERE recording_id=$1", r.PathValue("id"))
		}
		if input.Visibility != "" {
			for _, email := range input.Emails {
				if err != nil {
					break
				}
				tag, e := tx.Exec(r.Context(), "INSERT INTO recording_shares SELECT $1,id FROM app_users WHERE email=$2 ON CONFLICT DO NOTHING", r.PathValue("id"), strings.ToLower(strings.TrimSpace(email)))
				err = e
				if e == nil && tag.RowsAffected() == 0 {
					http.Error(w, "Email được chia sẻ chưa có tài khoản hoặc bị lặp", 400)
					return
				}
			}
		}
		if err == nil {
			err = tx.Commit(r.Context())
		}
		if err != nil {
			http.Error(w, "Không lưu được bài", 500)
			return
		}
		jsonResponse(w, 200, map[string]bool{"ok": true})
	})
	mux.HandleFunc("DELETE /api/recordings/{id}", func(w http.ResponseWriter, r *http.Request) {
		_, err := db.Exec(r.Context(), "UPDATE recordings SET deleted_at=now() WHERE id=$1", r.PathValue("id"))
		if err != nil {
			http.Error(w, "Không xóa được", 500)
			return
		}
		jsonResponse(w, 200, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /api/recordings/{id}/restore", func(w http.ResponseWriter, r *http.Request) {
		_, err := db.Exec(r.Context(), "UPDATE recordings SET deleted_at=NULL WHERE id=$1", r.PathValue("id"))
		if err != nil {
			http.Error(w, "Không khôi phục được", 500)
			return
		}
		jsonResponse(w, 200, map[string]bool{"ok": true})
	})
}
