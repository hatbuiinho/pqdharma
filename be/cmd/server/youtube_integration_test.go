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

func testYouTubeImport(t *testing.T, db *pgxpool.Pool) {
	mux := http.NewServeMux()
	registerYouTubeRoutes(mux, db)
	request := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/recordings/youtube", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	w := request(`{"url":"https://youtu.be/aaaS89avDVU?t=3","title":"Tên bài tự sửa"}`)
	if w.Code != 202 {
		t.Fatalf("import: %d %s", w.Code, w.Body.String())
	}
	var ids map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &ids); err != nil {
		t.Fatal(err)
	}
	var title, source, kind, status, audio, payload string
	err := db.QueryRow(context.Background(), `SELECT r.title,r.source_url,r.audio_file,j.kind,j.status,j.payload->>'youtubeUrl'
 FROM recordings r JOIN processing_jobs j ON j.recording_id=r.id WHERE r.id=$1 AND j.id=$2`, ids["recordingId"], ids["jobId"]).Scan(&title, &source, &audio, &kind, &status, &payload)
	if err != nil {
		t.Fatal(err)
	}
	if title != "Tên bài tự sửa" || source != "https://www.youtube.com/watch?v=aaaS89avDVU" || payload != source || audio != ids["recordingId"]+".mp3" || kind != "transcribe" || status != "queued" {
		t.Fatalf("unexpected import: %s %s %s %s %s %s", title, source, audio, kind, status, payload)
	}
	if w = request(`{"url":"https://www.youtube.com/watch?v=aaaS89avDVU","title":"duplicate"}`); w.Code != 409 {
		t.Fatalf("duplicate: %d %s", w.Code, w.Body.String())
	}
	if w = request(`{"url":"http://localhost/private","title":"bad"}`); w.Code != 400 {
		t.Fatalf("unsafe URL: %d", w.Code)
	}
	var count int
	if err = db.QueryRow(context.Background(), "SELECT count(*) FROM recordings WHERE source_url=$1", source).Scan(&count); err != nil || count != 1 {
		t.Fatalf("recordings=%d err=%v", count, err)
	}
}
