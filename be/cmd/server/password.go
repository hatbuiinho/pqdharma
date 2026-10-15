package main

import (
	"encoding/json"
	"net/http"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

func registerPassword(mux *http.ServeMux, db *pgxpool.Pool) {
	mux.HandleFunc("POST /api/auth/password", func(w http.ResponseWriter, r *http.Request) {
		u := currentUser(r)
		if u.ID == "" {
			http.Error(w, "Vui lòng đăng nhập", 401)
			return
		}
		var input struct {
			CurrentPassword string `json:"currentPassword"`
			NewPassword     string `json:"newPassword"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		if json.NewDecoder(r.Body).Decode(&input) != nil || len(input.CurrentPassword) == 0 || len(input.CurrentPassword) > 72 || utf8.RuneCountInString(input.NewPassword) < 12 || len(input.NewPassword) > 72 {
			http.Error(w, "Nhập mật khẩu hiện tại và mật khẩu mới từ 12 ký tự, tối đa 72 byte", 400)
			return
		}
		if input.CurrentPassword == input.NewPassword {
			http.Error(w, "Mật khẩu mới phải khác mật khẩu hiện tại", 400)
			return
		}
		if !allowLogin("change-password:" + u.ID) {
			http.Error(w, "Thử đổi mật khẩu quá nhiều. Chờ một phút rồi thử lại.", 429)
			return
		}
		tx, err := db.Begin(r.Context())
		if err != nil {
			http.Error(w, "Không đổi được mật khẩu", 500)
			return
		}
		defer tx.Rollback(r.Context())
		var oldHash string
		if err = tx.QueryRow(r.Context(), "SELECT password_hash FROM app_users WHERE id=$1 AND NOT disabled FOR UPDATE", u.ID).Scan(&oldHash); err != nil {
			http.Error(w, "Tài khoản không khả dụng", 401)
			return
		}
		if bcrypt.CompareHashAndPassword([]byte(oldHash), []byte(input.CurrentPassword)) != nil {
			http.Error(w, "Mật khẩu hiện tại không đúng", 400)
			return
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(input.NewPassword), bcrypt.DefaultCost)
		if err == nil {
			_, err = tx.Exec(r.Context(), "UPDATE app_users SET password_hash=$2 WHERE id=$1", u.ID, string(hash))
		}
		if err == nil {
			_, err = tx.Exec(r.Context(), "DELETE FROM user_sessions WHERE user_id=$1", u.ID)
		}
		if err == nil {
			err = tx.Commit(r.Context())
		}
		if err != nil {
			http.Error(w, "Không đổi được mật khẩu", 500)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "pq_session", Path: "/", MaxAge: -1, HttpOnly: true, Secure: secureCookie(), SameSite: http.SameSiteLaxMode})
		jsonResponse(w, 200, map[string]bool{"ok": true})
	})
}
