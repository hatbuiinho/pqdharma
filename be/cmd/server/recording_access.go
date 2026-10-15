package main

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

func registerRecordingAccess(mux *http.ServeMux, db *pgxpool.Pool) {
	mux.HandleFunc("GET /api/recordings/{id}/access", func(w http.ResponseWriter, r *http.Request) {
		var owner user
		var visibility string
		err := db.QueryRow(r.Context(), "SELECT COALESCE(r.owner_id,''),COALESCE(u.email,''),COALESCE(u.name,''),r.visibility FROM recordings r LEFT JOIN app_users u ON u.id=r.owner_id WHERE r.id=$1", r.PathValue("id")).Scan(&owner.ID, &owner.Email, &owner.Name, &visibility)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		owned := owner.ID != "" && owner.ID == currentUser(r).ID
		recipients := []user{}
		emails := []string{}
		if owned {
			rows, err := db.Query(r.Context(), "SELECT u.id,u.email,u.name,u.disabled FROM app_users u JOIN recording_shares s ON s.user_id=u.id WHERE s.recording_id=$1 ORDER BY u.email", r.PathValue("id"))
			if err != nil {
				http.Error(w, "Không tải được quyền truy cập", 500)
				return
			}
			defer rows.Close()
			for rows.Next() {
				var person user
				if err = rows.Scan(&person.ID, &person.Email, &person.Name, &person.Disabled); err != nil {
					http.Error(w, "Không tải được quyền truy cập", 500)
					return
				}
				recipients = append(recipients, person)
				emails = append(emails, person.Email)
			}
			if rows.Err() != nil {
				http.Error(w, "Không tải được quyền truy cập", 500)
				return
			}
		}
		jsonResponse(w, 200, map[string]any{"owned": owned, "visibility": visibility, "owner": owner, "users": recipients, "emails": emails})
	})
	mux.HandleFunc("PATCH /api/recordings/{id}/access", func(w http.ResponseWriter, r *http.Request) {
		// Keep ownership enforcement here as well as in authMiddleware.
		var owner string
		if err := db.QueryRow(r.Context(), "SELECT COALESCE(owner_id,'') FROM recordings WHERE id=$1", r.PathValue("id")).Scan(&owner); err != nil {
			http.NotFound(w, r)
			return
		}
		if owner == "" || owner != currentUser(r).ID {
			http.Error(w, "Chỉ chủ sở hữu được thay đổi quyền", 403)
			return
		}
		var input struct {
			Visibility string   `json:"visibility"`
			Emails     []string `json:"emails"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16384)
		if json.NewDecoder(r.Body).Decode(&input) != nil || len(input.Emails) > 100 || (input.Visibility != "private" && input.Visibility != "shared" && input.Visibility != "public") {
			http.Error(w, "Quyền truy cập không hợp lệ", 400)
			return
		}
		if input.Visibility == "private" && len(input.Emails) > 0 || input.Visibility == "shared" && len(input.Emails) == 0 {
			http.Error(w, "Danh sách người xem không phù hợp với quyền truy cập", 400)
			return
		}
		tx, err := db.Begin(r.Context())
		if err != nil {
			http.Error(w, "Không lưu được quyền", 500)
			return
		}
		defer tx.Rollback(r.Context())
		_, err = tx.Exec(r.Context(), "UPDATE recordings SET visibility=$2 WHERE id=$1", r.PathValue("id"), input.Visibility)
		if err == nil {
			_, err = tx.Exec(r.Context(), "DELETE FROM recording_shares WHERE recording_id=$1", r.PathValue("id"))
		}
		seen := map[string]bool{}
		for _, email := range input.Emails {
			if err != nil {
				break
			}
			email = strings.ToLower(strings.TrimSpace(email))
			if seen[email] {
				continue
			}
			seen[email] = true
			tag, e := tx.Exec(r.Context(), "INSERT INTO recording_shares SELECT $1,id FROM app_users WHERE email=$2 AND id<>$3 ON CONFLICT DO NOTHING", r.PathValue("id"), email, owner)
			err = e
			if e == nil && tag.RowsAffected() == 0 {
				http.Error(w, "Người được chia sẻ không hợp lệ. Hãy tải lại và chọn tài khoản có sẵn.", 400)
				return
			}
		}
		if err == nil {
			err = tx.Commit(r.Context())
		}
		if err != nil {
			http.Error(w, "Không lưu được quyền truy cập", 500)
			return
		}
		jsonResponse(w, 200, map[string]bool{"ok": true})
	})
}
