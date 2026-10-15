package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"
)

func registerAccountProfile(mux *http.ServeMux, db *pgxpool.Pool) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		actor := currentUser(r)
		if actor.ID == "" {
			http.Error(w, "Vui lòng đăng nhập", 401)
			return
		}
		id := actor.ID
		if target := r.PathValue("id"); target != "" {
			if !actor.Admin {
				http.Error(w, "Chỉ quản trị viên được sửa tài khoản khác", 403)
				return
			}
			id = target
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		var input struct {
			Name string `json:"name"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			http.Error(w, "Dữ liệu không hợp lệ", 400)
			return
		}
		input.Name = strings.TrimSpace(input.Name)
		if input.Name == "" || utf8.RuneCountInString(input.Name) > 100 {
			http.Error(w, "Tên cần từ 1 đến 100 ký tự", 400)
			return
		}
		tag, err := db.Exec(r.Context(), "UPDATE app_users SET name=$2 WHERE id=$1", id, input.Name)
		if err != nil {
			http.Error(w, "Không lưu được tên", 500)
			return
		}
		if tag.RowsAffected() == 0 {
			http.NotFound(w, r)
			return
		}
		jsonResponse(w, 200, map[string]string{"name": input.Name})
	}
	mux.HandleFunc("PATCH /api/auth/profile", handler)
	mux.HandleFunc("PATCH /api/users/{id}/profile", handler)
}
