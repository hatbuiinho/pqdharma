package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testRecordingSharing(t *testing.T, db *pgxpool.Pool) {
	ctx := context.Background()
	for _, id := range []string{"share-owner", "share-reader", "share-other"} {
		if _, err := db.Exec(ctx, "INSERT INTO app_users(id,email,name,password_hash) VALUES($1,$2,$1,'unused')", id, id+"@test.local"); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(ctx, "INSERT INTO user_sessions VALUES($1,$2,now()+interval '1 hour')", sessionHash(id), id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(ctx, "INSERT INTO sutras VALUES('share-test','Share test','en')"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "INSERT INTO recordings(id,sutra_id,title,audio_file,source_url,status,owner_id) VALUES('share-test','share-test','Share test','x.mp3','','ready','share-owner')"); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerManagement(mux, db)
	mux.HandleFunc("GET /api/recordings/{id}/probe", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	handler := authMiddleware(db, mux)
	request := func(method, path, body, identity string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if identity != "" {
			r.AddCookie(&http.Cookie{Name: "pq_session", Value: identity})
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s as %s: got %d want %d: %s", method, path, identity, w.Code, want, w.Body)
		}
		return w
	}
	access := "/api/recordings/share-test/access"
	probe := "/api/recordings/share-test/probe"
	request("GET", probe, "", "share-reader", 404)
	request("PATCH", access, `{"visibility":"shared","emails":["share-reader@test.local"]}`, "share-owner", 200)
	request("GET", probe, "", "share-reader", 200)
	request("GET", probe, "", "share-other", 404)
	request("PATCH", access, `{"visibility":"public","emails":[]}`, "share-reader", 403)
	// Sharing changes never rename the recording; title changes never revoke grants.
	request("PATCH", "/api/recordings/share-test", `{"title":"Renamed"}`, "share-owner", 200)
	var current struct {
		Visibility string
		Emails     []string
		Users      []user
		Owned      bool
	}
	w := request("GET", access, "", "share-owner", 200)
	if err := json.Unmarshal(w.Body.Bytes(), &current); err != nil || !current.Owned || current.Visibility != "shared" || len(current.Emails) != 1 {
		t.Fatalf("rename changed permissions: %s", w.Body)
	}
	request("PATCH", access, `{"visibility":"public","emails":["share-reader@test.local"]}`, "share-owner", 200)
	request("GET", probe, "", "", 200)
	w = request("GET", access, "", "share-reader", 200)
	json.Unmarshal(w.Body.Bytes(), &current)
	if current.Owned || len(current.Users) != 0 || len(current.Emails) != 0 {
		t.Fatal("viewer can inspect private recipient list")
	}
	request("PATCH", access, `{"visibility":"shared","emails":["missing@test.local"]}`, "share-owner", 400)
	request("GET", probe, "", "", 200) // Failed save must preserve public access.
	request("PATCH", access, `{"visibility":"shared","emails":["share-reader@test.local"]}`, "share-owner", 200)
	request("GET", probe, "", "", 404)
	request("GET", probe, "", "share-reader", 200)
	request("PATCH", access, `{"visibility":"private","emails":[]}`, "share-owner", 200)
	request("GET", probe, "", "share-reader", 404)
	var title string
	if err := db.QueryRow(ctx, "SELECT title FROM recordings WHERE id='share-test'").Scan(&title); err != nil || title != "Renamed" {
		t.Fatal("permissions changed recording title")
	}
}
