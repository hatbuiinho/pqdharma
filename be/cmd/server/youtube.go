package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var videoIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

func youtubeURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Port() != "" {
		return "", fmt.Errorf("Link YouTube không hợp lệ")
	}
	var id string
	switch strings.ToLower(u.Hostname()) {
	case "youtu.be":
		id = strings.Trim(u.Path, "/")
	case "youtube.com", "www.youtube.com", "m.youtube.com":
		if u.Path == "/watch" {
			id = u.Query().Get("v")
		} else {
			parts := strings.Split(strings.Trim(u.Path, "/"), "/")
			if len(parts) == 2 && (parts[0] == "shorts" || parts[0] == "embed") {
				id = parts[1]
			}
		}
	}
	if !videoIDPattern.MatchString(id) {
		return "", fmt.Errorf("Hãy nhập link một video YouTube hợp lệ")
	}
	return "https://www.youtube.com/watch?v=" + id, nil
}
func registerYouTubeRoutes(mux *http.ServeMux, db *pgxpool.Pool) {
	mux.HandleFunc("POST /api/youtube/preview", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		var input struct {
			URL string `json:"url"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			http.Error(w, "Dữ liệu không hợp lệ", 400)
			return
		}
		canonical, err := youtubeURL(input.URL)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, "GET", "https://www.youtube.com/oembed?format=json&url="+url.QueryEscape(canonical), nil)
		client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, "Không lấy được tên video; bạn có thể nhập tên thủ công", 502)
			return
		}
		defer resp.Body.Close()
		var info struct {
			Title string `json:"title"`
		}
		if resp.StatusCode != 200 || json.NewDecoder(http.MaxBytesReader(w, resp.Body, 65536)).Decode(&info) != nil || strings.TrimSpace(info.Title) == "" {
			http.Error(w, "Không lấy được tên video; bạn có thể nhập tên thủ công", 502)
			return
		}
		jsonResponse(w, 200, map[string]string{"title": string([]rune(info.Title)[:min(200, len([]rune(info.Title)))]), "url": canonical})
	})
	mux.HandleFunc("POST /api/recordings/youtube", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		var input struct {
			URL   string `json:"url"`
			Title string `json:"title"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			http.Error(w, "Dữ liệu không hợp lệ", 400)
			return
		}
		canonical, err := youtubeURL(input.URL)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		title := strings.TrimSpace(input.Title)
		if title == "" || len([]rune(title)) > 200 {
			http.Error(w, "Tên bài cần từ 1 đến 200 ký tự", 400)
			return
		}
		tx, err := db.Begin(r.Context())
		if err != nil {
			http.Error(w, "Không lưu được bài", 500)
			return
		}
		defer tx.Rollback(r.Context())
		// Serialize imports of this canonical URL, including concurrent requests.
		if _, err = tx.Exec(r.Context(), "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", canonical); err != nil {
			http.Error(w, "Không lưu được bài", 500)
			return
		}
		var existingTitle string
		err = tx.QueryRow(r.Context(), "SELECT s.title FROM recordings r JOIN sutras s ON s.id=r.sutra_id WHERE r.source_url=$1 AND COALESCE(r.owner_id,'')=$2 AND r.deleted_at IS NULL LIMIT 1", canonical, currentUser(r).ID).Scan(&existingTitle)
		if err == nil {
			http.Error(w, "Video này đã có trong thư viện: "+existingTitle, 409)
			return
		}
		if err != pgx.ErrNoRows {
			http.Error(w, "Không kiểm tra được bài đã nhập", 500)
			return
		}
		id, jobID := newID(), newID()
		payload, _ := json.Marshal(map[string]string{"youtubeUrl": canonical})
		_, err = tx.Exec(r.Context(), "INSERT INTO sutras VALUES($1,$2,'en')", id, title)
		if err == nil {
			_, err = tx.Exec(r.Context(), "INSERT INTO recordings(id,sutra_id,title,audio_file,source_url,status) VALUES($1,$1,$2,$3,$4,'processing')", id, title, id+".mp3", canonical)
		}
		if err == nil && currentUser(r).ID != "" {
			_, err = tx.Exec(r.Context(), "UPDATE recordings SET owner_id=$2 WHERE id=$1", id, currentUser(r).ID)
		}

		if err == nil {
			_, err = tx.Exec(r.Context(), "INSERT INTO processing_jobs(id,recording_id,kind,status,payload) VALUES($1,$2,'transcribe','queued',$3)", jobID, id, payload)
		}
		if err == nil {
			err = tx.Commit(r.Context())
		}
		if err != nil {
			http.Error(w, "Không lưu được bài", 500)
			return
		}
		jsonResponse(w, 202, map[string]string{"recordingId": id, "jobId": jobID})
	})
}
