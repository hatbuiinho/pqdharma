package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUserManagementGuards(t *testing.T) {
	mux := http.NewServeMux()
	registerAuth(mux, nil)
	for _, tc := range []struct {
		method, path, body string
		identity           user
		want               int
	}{
		{"GET", "/api/users", "", user{}, 401},
		{"PATCH", "/api/auth/profile", `{"name":"An"}`, user{}, 401},
		{"PATCH", "/api/auth/profile", `{"name":"   "}`, user{ID: "reader"}, 400},
		{"PATCH", "/api/auth/profile", `{"name":"` + strings.Repeat("界", 101) + `"}`, user{ID: "reader"}, 400},
		{"PATCH", "/api/users/other/profile", `{"name":"An"}`, user{ID: "reader"}, 403},
		{"POST", "/api/auth/password", `{}`, user{}, 401},
		{"POST", "/api/auth/password", `{"currentPassword":"old-password-123","newPassword":"short"}`, user{ID: "reader"}, 400},
		{"POST", "/api/auth/password", `{"currentPassword":"old-password-123","newPassword":"old-password-123"}`, user{ID: "reader"}, 400},
		{"PATCH", "/api/users/other/status", `{"disabled":true}`, user{ID: "reader"}, 403},
		{"PATCH", "/api/users/admin/status", `{"disabled":true}`, user{ID: "admin", Admin: true}, 400},
		{"PATCH", "/api/users/other/status", `{}`, user{ID: "admin", Admin: true}, 400},
		{"POST", "/api/users", `{"email":"test@example.com","password":"long-password"}`, user{ID: "reader"}, 403},
		{"PATCH", "/api/users/other", `{"password":"long-password"}`, user{ID: "reader"}, 403},
		{"PATCH", "/api/users/other", `{"password":"short"}`, user{ID: "admin", Admin: true}, 400},
		{"GET", "/api/users?q=" + strings.Repeat("a", 255), "", user{ID: "reader"}, 400},
	} {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		r = r.WithContext(context.WithValue(r.Context(), userKey{}, tc.identity))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("%s %s: got %d want %d", tc.method, tc.path, w.Code, tc.want)
		}
	}
}

func TestAllowedOrigin(t *testing.T) {
	for _, tc := range []struct {
		configured, origin string
		want               bool
	}{
		{"", "http://localhost:5173", true},
		{"http://localhost:3000", "http://localhost:5173", true},
		{"http://localhost:3000/", "http://127.0.0.1:5173", true},
		{"http://localhost:3000", "http://localhost:3000", true},
		{"http://localhost:3000", "http://localhost:5174", false},
		{"http://localhost:3000", "http://localhost:5173.evil.test", false},
		{"https://dharma.example", "http://localhost:5173", false},
		{"https://dharma.example", "https://dharma.example", true},
	} {
		t.Run(tc.configured+"/"+tc.origin, func(t *testing.T) {
			t.Setenv("PUBLIC_ORIGIN", tc.configured)
			if got := allowedOrigin(tc.origin); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
