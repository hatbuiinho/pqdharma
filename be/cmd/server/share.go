package main

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var shellMeta = regexp.MustCompile(`(?is)<title\b[^>]*>.*?</title>|<meta\b[^>]*\bname=["']description["'][^>]*>`)

func shareHTML(shell, title, description, canonical, image string) string {
	shell = shellMeta.ReplaceAllString(shell, "")
	escape := html.EscapeString
	tags := "<title>" + escape(title+" · PQ Dharma") + "</title>\n"
	for _, pair := range [][2]string{{"og:title", title}, {"og:description", description}, {"og:type", "website"}, {"og:url", canonical}, {"og:image", image}, {"og:image:alt", title}, {"og:site_name", "PQ Dharma"}, {"og:locale", "vi_VN"}} {
		tags += fmt.Sprintf("<meta property=%q content=\"%s\">\n", pair[0], escape(pair[1]))
	}
	for _, pair := range [][2]string{{"description", description}, {"twitter:card", "summary_large_image"}, {"twitter:title", title}, {"twitter:description", description}, {"twitter:image", image}} {
		tags += fmt.Sprintf("<meta name=%q content=\"%s\">\n", pair[0], escape(pair[1]))
	}
	tags += "<link rel=\"canonical\" href=\"" + escape(canonical) + "\">\n"
	return strings.Replace(shell, "</head>", tags+"</head>", 1)
}

func registerShareRoutes(mux *http.ServeMux, db *pgxpool.Pool) {
	client := &http.Client{Timeout: 10 * time.Second}
	mux.HandleFunc("GET /api/share/recordings/{id}", func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimRight(os.Getenv("PUBLIC_ORIGIN"), "/")
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "" {
			http.Error(w, "PUBLIC_ORIGIN must be configured as the public app origin", 503)
			return
		}
		id := r.PathValue("id")
		var title, language, hash string
		var duration int64
		err = db.QueryRow(r.Context(), `SELECT s.title,s.language,COALESCE(r.duration_ms,0),COALESCE(a.sha256,'') FROM recordings r JOIN sutras s ON s.id=r.sutra_id LEFT JOIN recording_assets a ON a.recording_id=r.id AND a.kind='thumbnails' WHERE r.id=$1`, id).Scan(&title, &language, &duration, &hash)
		if err != nil { // Distinguish missing recordings from database failures.
			if err == pgx.ErrNoRows {
				http.NotFound(w, r)
			} else {
				http.Error(w, "Không tải được bài nghe", 503)
			}
			return
		}
		image := origin + "/icons/icon-512.png"
		if hash != "" {
			image = origin + "/api/recordings/" + url.PathEscape(id) + "/thumbnail?v=" + url.QueryEscape(hash)
		} else if info, err := os.Stat(os.Getenv("UPLOAD_DIR") + "/" + id + ".thumbnail.jpg"); err == nil {
			image = fmt.Sprintf("%s/api/recordings/%s/thumbnail?v=%d", origin, url.PathEscape(id), info.ModTime().UnixNano())
		}
		if language == "en" {
			language = "Tiếng Anh"
		}
		description := language + " · Nghe và luyện từng lời cùng PQ Dharma."
		if duration > 0 {
			seconds := duration / 1000
			description = fmt.Sprintf("%s · %d:%02d · Nghe và luyện từng lời cùng PQ Dharma.", language, seconds/60, seconds%60)
		}
		shellOrigin := strings.TrimRight(os.Getenv("WEB_SHELL_ORIGIN"), "/")
		if shellOrigin == "" {
			shellOrigin = "http://pq-dharma-web:3000"
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		request, err := http.NewRequestWithContext(ctx, "GET", shellOrigin+"/index.html", nil)
		if err != nil {
			http.Error(w, "App shell unavailable", 503)
			return
		}
		response, err := client.Do(request)
		if err != nil {
			http.Error(w, "App shell unavailable", 503)
			return
		}
		defer response.Body.Close()
		shell, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
		if err != nil || response.StatusCode != 200 || !strings.Contains(string(shell), "</head>") {
			http.Error(w, "App shell unavailable", 503)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		io.WriteString(w, shareHTML(string(shell), title, description, origin+"/recordings/"+url.PathEscape(id), image))
	})
}
