package main

import (
	"context"
	"dharma-player/internal/core"
	"dharma-player/internal/phonetics"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestJobsDatabase(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL for isolated PostgreSQL integration checks")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "test_jobs_" + newID()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, file := range []string{"../../migrations/001_init.sql", "../../migrations/002_jobs.sql", "../../migrations/003_job_progress.sql", "../../migrations/004_ipa_voices.sql", "../../migrations/005_media.sql", "../../migrations/006_duration.sql", "../../migrations/007_users.sql", "../../migrations/008_user_status.sql", "../../migrations/009_user_name.sql"} {
		migration, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, string(migration)); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("MinIO storage", func(t *testing.T) { testStorageDatabase(t, db) })
	t.Run("User access", func(t *testing.T) { testUserAccess(t, db) })
	t.Run("User lifecycle", func(t *testing.T) { testUserLifecycle(t, db) })
	t.Run("Recording sharing", func(t *testing.T) { testRecordingSharing(t, db) })
	t.Run("PDF import", func(t *testing.T) { testPDFImport(t, db) })
	t.Run("YouTube import", func(t *testing.T) { testYouTubeImport(t, db) })
	generator := phonetics.Generator{Voice: "en-us", Run: func(context.Context, string, string) (string, error) { return "ipa", nil }}
	segments := []core.Segment{{Text: "one two", StartMs: 0, EndMs: 1000, Tokens: []core.Token{{Text: "one", StartMs: 0, EndMs: 400}, {Text: "two", StartMs: 500, EndMs: 1000}}}}
	result, _ := json.Marshal(segments)
	for _, id := range []string{"a", "b"} {
		if _, err = db.Exec(ctx, "INSERT INTO sutras VALUES($1,'test','en')", id); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, "INSERT INTO recordings(id,sutra_id,title,audio_file,source_url,status) VALUES($1,$1,'test','audio.mp3','','processing')", id); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, "INSERT INTO processing_jobs(id,recording_id,kind,status) VALUES($1,$2,'transcribe','aligned')", "job-"+id, id); err != nil {
			t.Fatal(err)
		}
		if err = finishJob(ctx, db, generator, id, "job-"+id, "transcribe", result, nil, 0); err != nil {
			t.Fatal(err)
		}
	}
	var progress int
	if err = db.QueryRow(ctx, "SELECT progress FROM processing_jobs WHERE id='job-a'").Scan(&progress); err != nil || progress != 100 {
		t.Fatalf("completed progress=%d error=%v", progress, err)
	}
	original, err := readTranscript(ctx, db, "a")
	if err != nil {
		t.Fatal(err)
	}
	other, err := readTranscript(ctx, db, "b")
	if err != nil {
		t.Fatal(err)
	}
	if original.Segments[0].ID == other.Segments[0].ID {
		t.Fatal("recording IDs collided")
	}
	sid := original.Segments[0].ID
	reviewed := []byte(`{"type":"ipa","text":"reviewed"}`)
	if _, err = db.Exec(ctx, "UPDATE token_ipa SET phonetic=$1 WHERE token_id=$2 AND voice='en-gb'", reviewed, original.Segments[0].Tokens[0].ID); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]string{"segmentId": sid})
	if _, err = db.Exec(ctx, "INSERT INTO processing_jobs(id,recording_id,kind,status) VALUES('edit','a','edit','aligned')"); err != nil {
		t.Fatal(err)
	}
	// A second active task must not race an edit on the same recording.
	if _, err = db.Exec(ctx, "INSERT INTO processing_jobs(id,recording_id,kind,status) VALUES('race','a','edit','queued')"); err == nil {
		t.Fatal("concurrent edit accepted")
	}
	segments[0].Text = "one three"
	segments[0].Tokens[1].Text = "three"
	result, _ = json.Marshal(segments)
	if err = finishJob(ctx, db, generator, "a", "edit", "edit", result, payload, 1); err != nil {
		t.Fatal(err)
	}
	updated, err := readTranscript(ctx, db, "a")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != 2 || updated.Segments[0].ID != sid || updated.Segments[0].Tokens[0].Phonetic.Text != "reviewed" || updated.Segments[0].Tokens[1].Text != "three" {
		t.Fatal("edit changed identities or reviewed IPA")
	}
	untouched, err := readTranscript(ctx, db, "b")
	if err != nil {
		t.Fatal(err)
	}
	if untouched.Segments[0].Text != "one two" {
		t.Fatal("editing one recording changed another")
	}
	if err = finishJob(ctx, db, generator, "a", "edit", "edit", result, payload, 1); err == nil {
		t.Fatal("stale revision was accepted")
	}
	after, _ := readTranscript(ctx, db, "a")
	if after.Segments[0].Text != "one three" {
		t.Fatal("failed edit changed live data")
	}
	// Imports produce one British version even if an old global generator was US.
	if other.DefaultIPAVoice != "en-gb" || !reflect.DeepEqual(other.IPAVoices, []string{"en-gb"}) || len(other.Segments[0].Tokens[0].Phonetics) != 1 {
		t.Fatal("import generated more than the British default")
	}
	mux := http.NewServeMux()
	registerIPARoutes(mux, db)
	registerImportRoutes(mux, db, t.TempDir())
	request := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	if w := request("POST", "/api/recordings/a/ipa", `{"voice":"bad"}`); w.Code != 400 {
		t.Fatalf("invalid voice accepted: %d", w.Code)
	}
	if w := request("PATCH", "/api/recordings/a/ipa/default", `{"voice":"en-us"}`); w.Code != 409 {
		t.Fatalf("missing IPA selected: %d", w.Code)
	}
	w := request("POST", "/api/recordings/a/ipa", `{"voice":"en-us"}`)
	if w.Code != 202 {
		t.Fatalf("enqueue IPA: %d %s", w.Code, w.Body.String())
	}
	var queued struct {
		JobID string `json:"jobId"`
	}
	json.Unmarshal(w.Body.Bytes(), &queued)
	if w := request("POST", "/api/recordings/a/ipa", `{"voice":"en-gb"}`); w.Code != 409 {
		t.Fatal("concurrent IPA accepted")
	}
	usPayload := []byte(`{"voice":"en-us"}`)
	usGenerator := phonetics.Generator{Run: func(_ context.Context, voice, text string) (string, error) {
		if voice != "en-us" {
			t.Fatalf("wrong requested voice %s", voice)
		}
		return "US " + text, nil
	}}
	if err = finishJob(ctx, db, usGenerator, "a", queued.JobID, "ipa", nil, usPayload, 2); err != nil {
		t.Fatal(err)
	}
	both, err := readTranscript(ctx, db, "a")
	if err != nil {
		t.Fatal(err)
	}
	if both.Revision != 2 || both.DefaultIPAVoice != "en-gb" || len(both.IPAVoices) != 2 || both.Segments[0].Tokens[0].Phonetics["en-us"].Text != "US one" || both.Segments[0].Tokens[0].Phonetic.Text != "reviewed" {
		t.Fatal("IPA generation changed default, revision or existing accent")
	}
	for i, token := range both.Segments[0].Tokens {
		old := after.Segments[0].Tokens[i]
		if token.ID != old.ID || token.Text != old.Text || token.StartMs != old.StartMs || token.EndMs != old.EndMs {
			t.Fatal("IPA generation changed text/timing")
		}
	}
	if w := request("PATCH", "/api/recordings/a/ipa/default", `{"voice":"en-us"}`); w.Code != 200 {
		t.Fatalf("set default: %d %s", w.Code, w.Body.String())
	}
	selected, _ := readTranscript(ctx, db, "a")
	unchanged, _ := readTranscript(ctx, db, "b")
	if selected.DefaultIPAVoice != "en-us" || selected.Segments[0].Tokens[0].Phonetic.Text != "US one" || unchanged.DefaultIPAVoice != "en-gb" {
		t.Fatal("default was not scoped to one recording")
	}
	// A failed regeneration publishes no partial result and can retry directly in the IPA queue.
	w = request("POST", "/api/recordings/a/ipa", `{"voice":"en-us"}`)
	json.Unmarshal(w.Body.Bytes(), &queued)
	failGenerator := phonetics.Generator{Run: func(_ context.Context, _, text string) (string, error) {
		if text == "three" {
			return "", errors.New("engine unavailable")
		}
		return "replacement", nil
	}}
	if err = finishJob(ctx, db, failGenerator, "a", queued.JobID, "ipa", nil, usPayload, 2); err == nil {
		t.Fatal("regeneration failure ignored")
	}
	failed, _ := readTranscript(ctx, db, "a")
	if !reflect.DeepEqual(selected, failed) {
		t.Fatal("failed generation published partial IPA")
	}
	db.Exec(ctx, "UPDATE processing_jobs SET status='failed' WHERE id=$1", queued.JobID)
	if w := request("POST", "/api/recordings/a/retry", ""); w.Code != 202 {
		t.Fatal("IPA retry failed")
	}
	var retryStatus string
	db.QueryRow(ctx, "SELECT status FROM processing_jobs WHERE id=$1", queued.JobID).Scan(&retryStatus)
	if retryStatus != "aligned" {
		t.Fatal("IPA retry entered transcription queue")
	}
	if err = finishJob(ctx, db, usGenerator, "a", queued.JobID, "ipa", nil, usPayload, 2); err != nil {
		t.Fatal(err)
	}
	// Regenerating GB keeps a reviewed pronunciation and the separate US version.
	w = request("POST", "/api/recordings/a/ipa", `{"voice":"en-gb"}`)
	json.Unmarshal(w.Body.Bytes(), &queued)
	gbGenerator := phonetics.Generator{Run: func(_ context.Context, voice, text string) (string, error) { return "GB " + text, nil }}
	if err = finishJob(ctx, db, gbGenerator, "a", queued.JobID, "ipa", nil, []byte(`{"voice":"en-gb"}`), 2); err != nil {
		t.Fatal(err)
	}
	regenerated, _ := readTranscript(ctx, db, "a")
	if regenerated.Segments[0].Tokens[0].Phonetics["en-gb"].Text != "reviewed" || regenerated.Segments[0].Tokens[1].Phonetics["en-gb"].Text != "GB three" || regenerated.Segments[0].Tokens[1].Phonetics["en-us"].Text != "US three" {
		t.Fatal("regeneration overwrote reviewed or other-accent IPA")
	}
	// Editing after choosing US generates only the recording default; other-accent corrections remain stored.
	payload, _ = json.Marshal(map[string]string{"segmentId": sid})
	segments[0].Text = "one four"
	segments[0].Tokens[1].Text = "four"
	result, _ = json.Marshal(segments)
	db.Exec(ctx, "INSERT INTO processing_jobs(id,recording_id,kind,status) VALUES('edit-us','a','edit','aligned')")
	if err = finishJob(ctx, db, usGenerator, "a", "edit-us", "edit", result, payload, 2); err != nil {
		t.Fatal(err)
	}
	editedUS, _ := readTranscript(ctx, db, "a")
	if editedUS.DefaultIPAVoice != "en-us" || !reflect.DeepEqual(editedUS.IPAVoices, []string{"en-us"}) || editedUS.Segments[0].Tokens[0].Phonetics["en-gb"].Text != "reviewed" || editedUS.Segments[0].Tokens[1].Phonetics["en-gb"] != nil {
		t.Fatal("edit mixed accents or discarded unchanged corrections")
	}
	// Legacy IPA remains unlabelled and untouched during migration/restarts.
	if _, err = db.Exec(ctx, `INSERT INTO sutras VALUES('legacy','legacy','en'); INSERT INTO recordings(id,sutra_id,title,audio_file,source_url,status,default_ipa_voice) VALUES('legacy','legacy','legacy','audio.mp3','','ready',NULL); INSERT INTO segments VALUES('legacy-segment','legacy',0,0,500,'hello'); INSERT INTO tokens VALUES('legacy-token','legacy-segment',0,'hello',0,500,false,'{"type":"ipa","text":"old IPA"}')`); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../migrations/004_ipa_voices.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	legacy, err := readTranscript(ctx, db, "legacy")
	if err != nil {
		t.Fatal(err)
	}
	if legacy.DefaultIPAVoice != "" || len(legacy.IPAVoices) != 0 || legacy.Segments[0].Tokens[0].Phonetic.Text != "old IPA" {
		t.Fatal("migration relabelled or replaced legacy IPA")
	}
	w = request("POST", "/api/recordings/legacy/ipa", `{"voice":"en-gb"}`)
	if w.Code != 202 {
		t.Fatal(w.Body.String())
	}
	json.Unmarshal(w.Body.Bytes(), &queued)
	if err = finishJob(ctx, db, gbGenerator, "legacy", queued.JobID, "ipa", nil, []byte(`{"voice":"en-gb"}`), 0); err != nil {
		t.Fatal(err)
	}
	named, err := readTranscript(ctx, db, "legacy")
	if err != nil {
		t.Fatal(err)
	}
	if named.DefaultIPAVoice != "en-gb" || named.Segments[0].Tokens[0].Phonetic.Text != "GB hello" {
		t.Fatal("legacy version was reused under a false accent label")
	}
	var oldIPA string
	if err = db.QueryRow(ctx, "SELECT phonetic->>'text' FROM tokens WHERE id='legacy-token'").Scan(&oldIPA); err != nil || oldIPA != "old IPA" {
		t.Fatal("original legacy IPA was deleted")
	}

}
