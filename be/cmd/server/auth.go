package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type user struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Admin    bool   `json:"admin"`
	Disabled bool   `json:"disabled"`
}
type userKey struct{}

func currentUser(r *http.Request) user { u, _ := r.Context().Value(userKey{}).(user); return u }
func sessionHash(token string) string {
	s := sha256.Sum256([]byte(token))
	return hex.EncodeToString(s[:])
}
func seedAdmin(ctx context.Context, db *pgxpool.Pool) error {
	email := strings.ToLower(strings.TrimSpace(os.Getenv("ADMIN_EMAIL")))
	password := os.Getenv("ADMIN_PASSWORD")
	if email == "" || password == "" {
		return nil
	}
	if len(password) < 12 {
		return fmt.Errorf("ADMIN_PASSWORD requires at least 12 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, "INSERT INTO app_users(id,email,password_hash,is_admin) VALUES($1,$2,$3,true) ON CONFLICT(email) DO NOTHING", newID(), email, string(hash))
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, "UPDATE recordings SET owner_id=(SELECT id FROM app_users WHERE email=$1) WHERE owner_id IS NULL", email)
	return err
}

// Vite development requests are proxied through the frontend but retain Origin.
func allowedOrigin(origin string) bool {
	expected := strings.TrimRight(os.Getenv("PUBLIC_ORIGIN"), "/")
	if expected == "" {
		expected = "http://localhost:3000"
	}
	if origin == expected {
		return true
	}
	switch expected {
	case "http://localhost:3000", "http://127.0.0.1:3000", "http://localhost:5173", "http://127.0.0.1:5173":
		return origin == "http://localhost:5173" || origin == "http://127.0.0.1:5173"
	}
	return false
}

func secureCookie() bool { return strings.HasPrefix(os.Getenv("PUBLIC_ORIGIN"), "https://") }
func authMiddleware(db *pgxpool.Pool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := user{}
		if c, err := r.Cookie("pq_session"); err == nil {
			db.QueryRow(r.Context(), "SELECT u.id,u.email,u.is_admin,u.name FROM user_sessions s JOIN app_users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.expires_at>now() AND NOT u.disabled", sessionHash(c.Value)).Scan(&u.ID, &u.Email, &u.Admin, &u.Name)
		}
		r = r.WithContext(context.WithValue(r.Context(), userKey{}, u))
		path := r.URL.Path
		if strings.HasPrefix(path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		if r.Method != "GET" && r.Method != "HEAD" && strings.HasPrefix(path, "/api/") {
			if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
				http.Error(w, "Cross-site request rejected", 403)
				return
			}
			origin := r.Header.Get("Origin")
			if origin != "" && !allowedOrigin(origin) {
				http.Error(w, "Origin không hợp lệ", 403)
				return
			}

			if path != "/api/auth/login" && u.ID == "" {
				http.Error(w, "Vui lòng đăng nhập", 401)
				return
			}
		}
		// Resolve recording access before handlers, including media and share HTML.
		prefix := "/api/recordings/"
		if strings.HasPrefix(path, "/api/share/recordings/") {
			prefix = "/api/share/recordings/"
		}
		if strings.HasPrefix(path, prefix) {
			parts := strings.Split(strings.TrimPrefix(path, prefix), "/")
			id, _ := url.PathUnescape(parts[0])
			if id != "youtube" && id != "pdf" && id != "" {
				var owner, visibility string
				var deleted, shared bool
				err := db.QueryRow(r.Context(), `SELECT COALESCE(owner_id,''),visibility,deleted_at IS NOT NULL,EXISTS(SELECT 1 FROM recording_shares WHERE recording_id=r.id AND user_id=$2) FROM recordings r WHERE id=$1`, id, u.ID).Scan(&owner, &visibility, &deleted, &shared)
				owns := u.ID != "" && owner == u.ID
				restore := r.Method == "POST" && len(parts) == 2 && parts[1] == "restore"
				allowed := owns || visibility == "public" || visibility == "shared" && shared
				if strings.HasPrefix(path, "/api/share/") {
					allowed = visibility == "public" || owns || visibility == "shared" && shared
				}
				if err != nil || !allowed || deleted && !(owns && restore) {
					http.NotFound(w, r)
					return
				}
				if len(parts) > 1 && parts[1] == "job" && !owns {
					jsonResponse(w, 200, map[string]any{"status": "ready", "progress": 100})
					return
				}
				if r.Method != "GET" && r.Method != "HEAD" && !owns {
					http.Error(w, "Chỉ chủ bài được chỉnh sửa", 403)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

var loginAttempts = struct {
	sync.Mutex
	counts map[string]int
	since  time.Time
}{counts: map[string]int{}, since: time.Now()}

func allowLogin(email string) bool {
	loginAttempts.Lock()
	defer loginAttempts.Unlock()
	if time.Since(loginAttempts.since) > time.Minute {
		loginAttempts.counts = map[string]int{}
		loginAttempts.since = time.Now()
	}
	if len(loginAttempts.counts) > 1000 {
		return false
	}
	loginAttempts.counts[email]++
	return loginAttempts.counts[email] <= 10
}
func registerAuth(mux *http.ServeMux, db *pgxpool.Pool) {
	registerUserStatus(mux, db)
	registerPassword(mux, db)
	registerAccountProfile(mux, db)
	mux.HandleFunc("GET /api/users", func(w http.ResponseWriter, r *http.Request) {
		u := currentUser(r)
		if u.ID == "" {
			http.Error(w, "Vui lòng đăng nhập", 401)
			return
		}
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		if len(q) > 254 {
			http.Error(w, "Từ khóa quá dài", 400)
			return
		}
		rows, err := db.Query(r.Context(), "SELECT id,email,is_admin,disabled,name FROM app_users WHERE (strpos(lower(email),lower($1))>0 OR strpos(lower(name),lower($1))>0) AND ($2 OR NOT disabled) ORDER BY email LIMIT 100", q, u.Admin && r.URL.Query().Get("include_disabled") == "true")
		if err != nil {
			http.Error(w, "Không tải được tài khoản", 500)
			return
		}
		defer rows.Close()
		users := []user{}
		for rows.Next() {
			var item user
			if err := rows.Scan(&item.ID, &item.Email, &item.Admin, &item.Disabled, &item.Name); err != nil {
				http.Error(w, "Không tải được tài khoản", 500)
				return
			}
			if !u.Admin {
				item.Admin = false
			}
			users = append(users, item)
		}
		if rows.Err() != nil {
			http.Error(w, "Không tải được tài khoản", 500)
			return
		}
		jsonResponse(w, 200, users)
	})
	mux.HandleFunc("PATCH /api/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !currentUser(r).Admin {
			http.Error(w, "Chỉ admin được quản lý user", 403)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		var input struct{ Password string }
		if json.NewDecoder(r.Body).Decode(&input) != nil || len(input.Password) < 12 || len(input.Password) > 72 {
			http.Error(w, "Mật khẩu cần 12–72 ký tự", 400)
			return
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
		if err != nil {
			http.Error(w, "Không cập nhật được mật khẩu", 500)
			return
		}
		tx, err := db.Begin(r.Context())
		if err != nil {
			http.Error(w, "Không cập nhật được mật khẩu", 500)
			return
		}
		defer tx.Rollback(r.Context())
		tag, err := tx.Exec(r.Context(), "UPDATE app_users SET password_hash=$2 WHERE id=$1", r.PathValue("id"), string(hash))
		if err != nil {
			http.Error(w, "Không cập nhật được mật khẩu", 500)
			return
		}
		if tag.RowsAffected() == 0 {
			http.NotFound(w, r)
			return
		}
		if _, err = tx.Exec(r.Context(), "DELETE FROM user_sessions WHERE user_id=$1", r.PathValue("id")); err == nil {
			err = tx.Commit(r.Context())
		}
		if err != nil {
			http.Error(w, "Không cập nhật được mật khẩu", 500)
			return
		}
		jsonResponse(w, 200, map[string]bool{"ok": true})
	})

	mux.HandleFunc("GET /api/auth/me", func(w http.ResponseWriter, r *http.Request) {
		u := currentUser(r)
		if u.ID == "" {
			jsonResponse(w, 200, map[string]any{"user": nil})
			return
		}
		jsonResponse(w, 200, map[string]any{"user": u})
	})
	mux.HandleFunc("POST /api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		var input struct{ Email, Password string }
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			http.Error(w, "Dữ liệu không hợp lệ", 400)
			return
		}
		if !allowLogin(strings.ToLower(strings.TrimSpace(input.Email))) {
			http.Error(w, "Thử đăng nhập quá nhiều. Chờ một phút rồi thử lại.", 429)
			return
		}
		var u user
		var hash string
		err := db.QueryRow(r.Context(), "SELECT id,email,is_admin,password_hash,name FROM app_users WHERE email=$1 AND NOT disabled", strings.ToLower(strings.TrimSpace(input.Email))).Scan(&u.ID, &u.Email, &u.Admin, &hash, &u.Name)
		if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(input.Password)) != nil {
			time.Sleep(300 * time.Millisecond)
			http.Error(w, "Email hoặc mật khẩu không đúng", 401)
			return
		}
		token := newID() + newID()
		_, err = db.Exec(r.Context(), "INSERT INTO user_sessions VALUES($1,$2,$3)", sessionHash(token), u.ID, time.Now().Add(7*24*time.Hour))
		if err != nil {
			http.Error(w, "Không đăng nhập được", 500)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "pq_session", Value: token, Path: "/", HttpOnly: true, Secure: secureCookie(), SameSite: http.SameSiteLaxMode, MaxAge: 604800})
		jsonResponse(w, 200, map[string]any{"user": u})
	})
	mux.HandleFunc("POST /api/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		if c, e := r.Cookie("pq_session"); e == nil {
			db.Exec(r.Context(), "DELETE FROM user_sessions WHERE token_hash=$1", sessionHash(c.Value))
		}
		http.SetCookie(w, &http.Cookie{Name: "pq_session", Path: "/", MaxAge: -1, HttpOnly: true, Secure: secureCookie(), SameSite: http.SameSiteLaxMode})
		jsonResponse(w, 200, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /api/users", func(w http.ResponseWriter, r *http.Request) {
		if !currentUser(r).Admin {
			http.Error(w, "Chỉ admin được tạo user", 403)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		var input struct {
			Email, Password, Name string
			Admin                 bool
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || !strings.Contains(input.Email, "@") || len(input.Email) > 254 || utf8.RuneCountInString(strings.TrimSpace(input.Name)) > 100 || len(input.Password) < 12 || len(input.Password) > 72 {
			http.Error(w, "Email hợp lệ và mật khẩu 12–72 ký tự", 400)
			return
		}
		input.Name = strings.TrimSpace(input.Name)
		if input.Name == "" {
			input.Name = strings.Split(strings.TrimSpace(input.Email), "@")[0]
			input.Name = string([]rune(input.Name)[:min(100, len([]rune(input.Name)))])
		}
		hash, _ := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
		_, err := db.Exec(r.Context(), "INSERT INTO app_users(id,email,password_hash,is_admin,name) VALUES($1,$2,$3,$4,$5)", newID(), strings.ToLower(strings.TrimSpace(input.Email)), string(hash), input.Admin, input.Name)
		if err != nil {
			http.Error(w, "Không tạo được user; email có thể đã tồn tại", 409)
			return
		}
		jsonResponse(w, 201, map[string]bool{"ok": true})
	})
}

func libraryScope(r *http.Request) string {
	scope := r.URL.Query().Get("scope")
	if scope == "shared" || scope == "public" || scope == "trash" {
		return scope
	}
	if currentUser(r).ID == "" {
		return "public"
	}
	return "mine"
}
