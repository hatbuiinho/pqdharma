package main

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testUserAccess(t *testing.T, db *pgxpool.Pool) {
	ctx := context.Background()
	for _, id := range []string{"owner", "reader", "stranger"} {
		_, err := db.Exec(ctx, "INSERT INTO app_users(id,email,password_hash,is_admin,created_at) VALUES($1,$2,'unused',false,now())", id, id+"@test.local")
		if err != nil {
			t.Fatal(err)
		}
		db.Exec(ctx, "INSERT INTO user_sessions VALUES($1,$2,now()+interval '1 hour')", sessionHash(id), id)
	}
	db.Exec(ctx, "INSERT INTO sutras VALUES('private-test','Private title','en')")
	db.Exec(ctx, "INSERT INTO recordings(id,sutra_id,title,audio_file,source_url,status,owner_id) VALUES('private-test','private-test','Private title','x.mp3','','ready','owner')")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/recordings/{id}/audio", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("PATCH /api/recordings/{id}", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	handler := authMiddleware(db, mux)
	check := func(method, identity string, want int) {
		t.Helper()
		r := httptest.NewRequest(method, "/api/recordings/private-test/audio", nil)
		if method == "PATCH" {
			r = httptest.NewRequest(method, "/api/recordings/private-test", nil)
		}
		if identity != "" {
			r.AddCookie(&http.Cookie{Name: "pq_session", Value: identity})
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s user=%s got %d want %d", method, identity, w.Code, want)
		}
	}
	check("GET", "", 404)
	check("GET", "owner", 200)
	check("GET", "reader", 404)
	db.Exec(ctx, "UPDATE recordings SET visibility='shared' WHERE id='private-test'")
	db.Exec(ctx, "INSERT INTO recording_shares VALUES('private-test','reader')")
	check("GET", "reader", 200)
	check("GET", "stranger", 404)
	check("PATCH", "reader", 403)
	check("PATCH", "owner", 200)
	db.Exec(ctx, "UPDATE recordings SET visibility='public' WHERE id='private-test'")
	check("GET", "", 200)
	db.Exec(ctx, "UPDATE recordings SET deleted_at=now() WHERE id='private-test'")
	check("GET", "", 404)
	check("GET", "owner", 404)
}
