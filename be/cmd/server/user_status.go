package main

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

func registerUserStatus(mux *http.ServeMux, db *pgxpool.Pool) {
	mux.HandleFunc("PATCH /api/users/{id}/status", func(w http.ResponseWriter, r *http.Request) {
		actor := currentUser(r)
		if !actor.Admin {
			http.Error(w, "Chỉ quản trị viên được thay đổi trạng thái tài khoản", 403)
			return
		}
		var input struct {
			Disabled *bool `json:"disabled"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		if json.NewDecoder(r.Body).Decode(&input) != nil || input.Disabled == nil {
			http.Error(w, "Trạng thái không hợp lệ", 400)
			return
		}
		if r.PathValue("id") == actor.ID {
			http.Error(w, "Bạn không thể vô hiệu hóa chính mình", 400)
			return
		}
		tx, err := db.Begin(r.Context())
		if err != nil {
			http.Error(w, "Không cập nhật được trạng thái", 500)
			return
		}
		defer tx.Rollback(r.Context())
		// Serialize status changes, then recheck the caller to prevent two admins
		// concurrently disabling each other and leaving no active administrator.
		if _, err = tx.Exec(r.Context(), "LOCK TABLE app_users IN SHARE ROW EXCLUSIVE MODE"); err != nil {
			http.Error(w, "Không cập nhật được trạng thái", 500)
			return
		}
		var authorized bool
		if err = tx.QueryRow(r.Context(), "SELECT is_admin AND NOT disabled FROM app_users WHERE id=$1", actor.ID).Scan(&authorized); err != nil || !authorized {
			http.Error(w, "Tài khoản không còn quyền quản trị", 403)
			return
		}
		tag, err := tx.Exec(r.Context(), "UPDATE app_users SET disabled=$2 WHERE id=$1", r.PathValue("id"), *input.Disabled)
		if err != nil {
			http.Error(w, "Không cập nhật được trạng thái", 500)
			return
		}
		if tag.RowsAffected() == 0 {
			http.NotFound(w, r)
			return
		}
		if *input.Disabled {
			_, err = tx.Exec(r.Context(), "DELETE FROM user_sessions WHERE user_id=$1", r.PathValue("id"))
		}
		if err == nil {
			err = tx.Commit(r.Context())
		}
		if err != nil {
			http.Error(w, "Không cập nhật được trạng thái", 500)
			return
		}
		jsonResponse(w, 200, map[string]bool{"ok": true})
	})
}
