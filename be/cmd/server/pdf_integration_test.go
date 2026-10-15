package main

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testPDFImport(t *testing.T, db *pgxpool.Pool) {
	mux := http.NewServeMux()
	registerPDFRoutes(mux, db, t.TempDir(), nil)
	body := `{"title":"PDF test","filename":"sample.pdf","turns":[{"speaker":"woman","text":"Hello."},{"speaker":"man","text":"Good afternoon."}],"voices":{"woman":"bf_emma","man":"bm_george"},"speed":1,"gap":0.5}`
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/recordings/pdf", strings.NewReader(body)))
	if w.Code != 202 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var ids map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &ids); err != nil {
		t.Fatal(err)
	}
	var text, speaker, voice, status string
	err := db.QueryRow(context.Background(), `SELECT payload->'pdfTts'->'turns'->0->>'text',payload->'pdfTts'->'turns'->1->>'speaker',payload->'pdfTts'->'voices'->>'man',status FROM processing_jobs WHERE id=$1`, ids["jobId"]).Scan(&text, &speaker, &voice, &status)
	if err != nil || text != "Hello." || speaker != "man" || voice != "bm_george" || status != "queued" {
		t.Fatalf("queue: %s %s %s %s %v", text, speaker, voice, status, err)
	}
	// Finish the test job so the shared integration suite has no unexpected queued work.
	if _, err := db.Exec(context.Background(), "UPDATE processing_jobs SET status='failed' WHERE id=$1", ids["jobId"]); err != nil {
		t.Fatal(err)
	}
}
