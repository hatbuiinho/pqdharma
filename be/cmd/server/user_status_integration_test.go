package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

func testUserLifecycle(t *testing.T, db *pgxpool.Pool) {
	ctx := context.Background()
	hash, err := bcrypt.GenerateFromPassword([]byte("test-password-123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, "INSERT INTO app_users(id,email,password_hash,is_admin) VALUES('lifecycle-admin','lifecycle-admin@test.local',$1,true)", string(hash))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerAuth(mux, db)
	request := func(method, path, body string, identity user) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r = r.WithContext(context.WithValue(r.Context(), userKey{}, identity))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	actor := user{ID: "lifecycle-admin", Admin: true}
	w := request("POST", "/api/users", `{"email":"second-admin@test.local","password":"test-password-123","admin":true,"name":"Minh An"}`, actor)
	if w.Code != 201 {
		t.Fatalf("create admin: %d %s", w.Code, w.Body)
	}
	var id string
	var admin bool
	if err = db.QueryRow(ctx, "SELECT id,is_admin FROM app_users WHERE email='second-admin@test.local'").Scan(&id, &admin); err != nil || !admin {
		t.Fatalf("new admin not persisted: %v", err)
	}
	_, err = db.Exec(ctx, "INSERT INTO user_sessions VALUES($1,$2,now()+interval '1 hour')", sessionHash("lifecycle-token"), id)
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/users/" + id + "/status"
	w = request("PATCH", path, `{"disabled":true}`, actor)
	if w.Code != 200 {
		t.Fatalf("disable: %d %s", w.Code, w.Body)
	}
	var count int
	db.QueryRow(ctx, "SELECT count(*) FROM user_sessions WHERE user_id=$1", id).Scan(&count)
	if count != 0 {
		t.Fatal("disabled user retains sessions")
	}
	login := `{"email":"second-admin@test.local","password":"test-password-123"}`
	if w = request("POST", "/api/auth/login", login, user{}); w.Code != 401 {
		t.Fatalf("disabled login: %d", w.Code)
	}
	// Even a stale session row cannot authenticate a disabled account.
	db.Exec(ctx, "INSERT INTO user_sessions VALUES($1,$2,now()+interval '1 hour')", sessionHash("stale-token"), id)
	r := httptest.NewRequest("GET", "/api/auth/me", nil)
	r.AddCookie(&http.Cookie{Name: "pq_session", Value: "stale-token"})
	w = httptest.NewRecorder()
	authMiddleware(db, mux).ServeHTTP(w, r)
	var me struct {
		User *user `json:"user"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &me); err != nil || me.User != nil {
		t.Fatal("disabled session authenticated")
	}
	// A stale admin identity must not be able to disable the remaining admin.
	if w = request("PATCH", "/api/users/lifecycle-admin/status", `{"disabled":true}`, user{ID: id, Admin: true}); w.Code != 403 {
		t.Fatalf("disabled actor: %d", w.Code)
	}
	w = request("GET", "/api/users?q=second-admin", "", user{ID: "reader"})
	var list []user
	if json.Unmarshal(w.Body.Bytes(), &list) != nil || len(list) != 0 {
		t.Fatal("disabled user exposed in sharing directory")
	}
	if w = request("PATCH", path, `{"disabled":false}`, actor); w.Code != 200 {
		t.Fatalf("reactivate: %d", w.Code)
	}
	if w = request("POST", "/api/auth/login", login, user{}); w.Code != 200 {
		t.Fatalf("reactivated login: %d", w.Code)
	}
	// Ordinary users can change only their own password after verifying the old one.
	if _, err = db.Exec(ctx, "INSERT INTO app_users(id,email,password_hash) VALUES('self-service','self-service@test.local',$1)", string(hash)); err != nil {
		t.Fatal(err)
	}
	self := user{ID: "self-service"}
	for _, token := range []string{"self-one", "self-two"} {
		if _, err = db.Exec(ctx, "INSERT INTO user_sessions VALUES($1,$2,now()+interval '1 hour')", sessionHash(token), self.ID); err != nil {
			t.Fatal(err)
		}
	}
	w = request("POST", "/api/auth/password", `{"currentPassword":"wrong-password","newPassword":"changed-password-123"}`, self)
	if w.Code != 400 {
		t.Fatalf("wrong current password: %d", w.Code)
	}
	var stored string
	if err = db.QueryRow(ctx, "SELECT password_hash FROM app_users WHERE id=$1", self.ID).Scan(&stored); err != nil || stored != string(hash) {
		t.Fatal("incorrect password changed account")
	}
	if err = db.QueryRow(ctx, "SELECT count(*) FROM user_sessions WHERE user_id=$1", self.ID).Scan(&count); err != nil || count != 2 {
		t.Fatal("incorrect password revoked sessions")
	}
	w = request("POST", "/api/auth/password", `{"currentPassword":"test-password-123","newPassword":"changed-password-123"}`, self)
	if w.Code != 200 {
		t.Fatalf("self change: %d %s", w.Code, w.Body)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "pq_session" || cookies[0].MaxAge != -1 {
		t.Fatal("session cookie not cleared")
	}
	if err = db.QueryRow(ctx, "SELECT count(*) FROM user_sessions WHERE user_id=$1", self.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("password change retained sessions")
	}
	if w = request("POST", "/api/auth/login", `{"email":"self-service@test.local","password":"test-password-123"}`, user{}); w.Code != 401 {
		t.Fatal("old password still works")
	}
	if w = request("POST", "/api/auth/login", `{"email":"self-service@test.local","password":"changed-password-123"}`, user{}); w.Code != 200 {
		t.Fatalf("new password login: %d", w.Code)
	}

	w = request("PATCH", "/api/auth/profile", `{"name":"  Nguyễn An  ","id":"lifecycle-admin","admin":true}`, self)
	if w.Code != 200 {
		t.Fatalf("self name change: %d %s", w.Code, w.Body)
	}
	var profile user
	if err = db.QueryRow(ctx, "SELECT name,email,is_admin FROM app_users WHERE id=$1", self.ID).Scan(&profile.Name, &profile.Email, &profile.Admin); err != nil || profile.Name != "Nguyễn An" || profile.Email != "self-service@test.local" || profile.Admin {
		t.Fatalf("profile update changed identity: %+v %v", profile, err)
	}
	if w = request("PATCH", "/api/users/"+self.ID+"/profile", `{"name":"Tên quản trị sửa"}`, actor); w.Code != 200 {
		t.Fatalf("admin name change: %d", w.Code)
	}
	w = request("GET", "/api/users?q="+"Minh%20An", "", actor)
	var named []user
	if err = json.Unmarshal(w.Body.Bytes(), &named); err != nil || len(named) != 1 || named[0].ID != id || named[0].Name != "Minh An" {
		t.Fatalf("name search: %s %v", w.Body, err)
	}

	// Concurrent cross-disable must leave one of the two administrators active.
	start := make(chan struct{})
	codes := make(chan int, 2)
	for _, pair := range []struct {
		actor  user
		target string
	}{
		{actor, id}, {user{ID: id, Admin: true}, actor.ID},
	} {
		go func(identity user, target string) {
			<-start
			r := httptest.NewRequest("PATCH", "/api/users/"+target+"/status", strings.NewReader(`{"disabled":true}`))
			r = r.WithContext(context.WithValue(r.Context(), userKey{}, identity))
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			codes <- w.Code
		}(pair.actor, pair.target)
	}
	close(start)
	first, second := <-codes, <-codes
	if !((first == 200 && second == 403) || (first == 403 && second == 200)) {
		t.Fatalf("cross-disable responses: %d, %d", first, second)
	}
	if err = db.QueryRow(ctx, "SELECT count(*) FROM app_users WHERE id=ANY($1) AND is_admin AND NOT disabled", []string{actor.ID, id}).Scan(&count); err != nil || count != 1 {
		t.Fatalf("active admins after cross-disable: %d, %v", count, err)
	}

}
