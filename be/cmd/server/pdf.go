package main

import (
	"bytes"
	"context"
	"dharma-player/internal/objectstore"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type pdfTurn struct {
	Speaker string `json:"speaker"`
	Text    string `json:"text"`
}
type pdfImport struct {
	Thumbnail []byte            `json:"thumbnail,omitempty"`
	Title     string            `json:"title"`
	Filename  string            `json:"filename"`
	Turns     []pdfTurn         `json:"turns"`
	Voices    map[string]string `json:"voices"`
	Speed     float64           `json:"speed"`
	Gap       float64           `json:"gap"`
}

var speakerLine = regexp.MustCompile(`(?i)^(woman|man|narrator)\s*:\s*(.*)$`)
var pdfVoices = map[string]bool{"bf_emma": true, "bf_isabella": true, "bm_george": true, "bm_lewis": true, "af_heart": true, "am_michael": true}

func parsePDFTurns(text string) []pdfTurn {
	text = strings.NewReplacer("\u200b", "", "\ufeff", "", "\r", "", "\f", "\n").Replace(text)
	turns := []pdfTurn{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			continue
		}
		if match := speakerLine.FindStringSubmatch(line); match != nil {
			turns = append(turns, pdfTurn{Speaker: strings.ToLower(match[1]), Text: match[2]})
		} else if len(turns) > 0 {
			turns[len(turns)-1].Text += " " + line
		} else {
			turns = append(turns, pdfTurn{Speaker: "narrator", Text: line})
		}
	}
	// Split long prose into editable, bounded synthesis turns without losing words.
	chunks := []pdfTurn{}
	for _, turn := range turns {
		runes := []rune(turn.Text)
		for len(runes) > 3000 {
			cut := 3000
			for i := 2999; i > 1500; i-- {
				if runes[i] == ' ' {
					cut = i
					break
				}
			}
			chunks = append(chunks, pdfTurn{Speaker: turn.Speaker, Text: strings.TrimSpace(string(runes[:cut]))})
			runes = runes[cut:]
		}
		chunks = append(chunks, pdfTurn{Speaker: turn.Speaker, Text: strings.TrimSpace(string(runes))})
	}
	return chunks
}

func validatePDFImport(input *pdfImport) error {
	if len(input.Thumbnail) > 250000 {
		return fmt.Errorf("Ảnh bìa quá lớn")
	}
	if len(input.Thumbnail) > 0 {
		config, err := jpeg.DecodeConfig(bytes.NewReader(input.Thumbnail))
		if err != nil || config.Width > 1280 || config.Height > 1280 {
			return fmt.Errorf("Ảnh bìa không hợp lệ")
		}
	}
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" || len([]rune(input.Title)) > 200 {
		return fmt.Errorf("Tên bài cần từ 1 đến 200 ký tự")
	}
	if len(input.Turns) == 0 || len(input.Turns) > 300 {
		return fmt.Errorf("Cần từ 1 đến 300 lượt đọc")
	}
	if input.Speed < .75 || input.Speed > 1.25 || input.Gap < 0 || input.Gap > 2 {
		return fmt.Errorf("Tốc độ từ 0.75 đến 1.25; khoảng nghỉ từ 0 đến 2 giây")
	}
	total := 0
	for i := range input.Turns {
		turn := &input.Turns[i]
		turn.Text = strings.TrimSpace(turn.Text)
		if turn.Speaker != "woman" && turn.Speaker != "man" && turn.Speaker != "narrator" {
			return fmt.Errorf("Người nói không hợp lệ")
		}
		n := len([]rune(turn.Text))
		total += n
		if n == 0 || n > 4000 {
			return fmt.Errorf("Mỗi lượt đọc cần từ 1 đến 4000 ký tự")
		}
		if !pdfVoices[input.Voices[turn.Speaker]] {
			return fmt.Errorf("Hãy chọn giọng cho từng người nói")
		}
	}
	if total > 60000 {
		return fmt.Errorf("Nội dung tối đa 60.000 ký tự; hãy chia PDF thành các phần nhỏ")
	}
	return nil
}

func registerPDFRoutes(mux *http.ServeMux, db *pgxpool.Pool, dir string, store *objectstore.Store) {
	mux.HandleFunc("POST /api/pdf/preview", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 21<<20)
		if err := r.ParseMultipartForm(2 << 20); err != nil {
			http.Error(w, "PDF tối đa 20 MB", 400)
			return
		}
		defer r.MultipartForm.RemoveAll()
		src, h, err := r.FormFile("pdf")
		if err != nil {
			http.Error(w, "Chọn file PDF", 400)
			return
		}
		defer src.Close()
		if strings.ToLower(filepath.Ext(h.Filename)) != ".pdf" || h.Size > 20_000_000 {
			http.Error(w, "Chọn PDF tối đa 20 MB", 400)
			return
		}
		dir, err := os.MkdirTemp("", "pq-pdf-*")
		if err != nil {
			http.Error(w, "Không đọc được PDF", 500)
			return
		}
		defer os.RemoveAll(dir)
		path := filepath.Join(dir, "input.pdf")
		file, err := os.Create(path)
		if err != nil {
			http.Error(w, "Không đọc được PDF", 500)
			return
		}
		_, copyErr := io.Copy(file, src)
		closeErr := file.Close()
		if copyErr != nil || closeErr != nil {
			http.Error(w, "Không đọc được PDF", 400)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		textPath := filepath.Join(dir, "text.txt")
		// Read at most 51 pages; never silently truncate a longer PDF.
		cmd := exec.CommandContext(ctx, "pdftotext", "-f", "1", "-l", "51", "-layout", "-enc", "UTF-8", path, textPath)
		if err := cmd.Run(); err != nil {
			http.Error(w, "Không đọc được PDF. Hãy chọn PDF không khóa và có lớp văn bản.", 400)
			return
		}
		info, err := os.Stat(textPath)
		if err != nil || info.Size() > 512000 {
			http.Error(w, "Nội dung PDF quá dài; hãy chia thành các phần nhỏ", 400)
			return
		}
		raw, err := os.ReadFile(textPath)
		if err != nil {
			http.Error(w, "Không đọc được nội dung PDF", 500)
			return
		}
		pages := strings.Count(string(raw), "\f")
		if pages > 50 {
			http.Error(w, "PDF tối đa 50 trang; hãy chia thành các phần nhỏ", 400)
			return
		}
		turns := parsePDFTurns(string(raw))
		if len(turns) == 0 {
			http.Error(w, "PDF không có văn bản đọc được. Bản đầu chưa hỗ trợ OCR cho PDF scan.", 400)
			return
		}
		if len(turns) > 300 || len([]rune(string(raw))) > 60000 {
			http.Error(w, "PDF quá dài; giới hạn 300 lượt đọc và 60.000 ký tự", 400)
			return
		}
		var thumbnail []byte
		prefix := filepath.Join(dir, "cover")
		if err := exec.CommandContext(ctx, "pdftoppm", "-f", "1", "-singlefile", "-scale-to", "640", "-jpeg", path, prefix).Run(); err == nil {
			if image, err := os.ReadFile(prefix + ".jpg"); err == nil && len(image) <= 250000 {
				thumbnail = image
			}
		}
		title := strings.TrimSuffix(filepath.Base(h.Filename), filepath.Ext(h.Filename))
		jsonResponse(w, 200, map[string]any{"title": title, "filename": filepath.Base(h.Filename), "pages": pages, "turns": turns, "thumbnail": thumbnail})
	})
	mux.HandleFunc("POST /api/recordings/pdf", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		var input pdfImport
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			http.Error(w, "Dữ liệu PDF không hợp lệ", 400)
			return
		}
		if err := validatePDFImport(&input); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		id, jobID := newID(), newID()
		thumbnail := input.Thumbnail
		input.Thumbnail = nil
		payload, err := json.Marshal(map[string]any{"pdfTts": input})
		if err != nil {
			http.Error(w, "Dữ liệu không hợp lệ", 400)
			return
		}
		tx, err := db.Begin(r.Context())
		if err != nil {
			http.Error(w, "Không lưu được bài", 500)
			return
		}
		defer tx.Rollback(r.Context())
		if _, err = tx.Exec(r.Context(), "INSERT INTO sutras VALUES($1,$2,'en')", id, input.Title); err == nil {
			_, err = tx.Exec(r.Context(), "INSERT INTO recordings(id,sutra_id,title,audio_file,source_url,status) VALUES($1,$1,$2,$3,'','processing')", id, input.Title, id+".mp3")
		}
		if err == nil {
			_, err = tx.Exec(r.Context(), "INSERT INTO processing_jobs(id,recording_id,kind,status,payload) VALUES($1,$2,'transcribe','queued',$3)", jobID, id, payload)
		}
		if err == nil {
			err = tx.Commit(r.Context())
		}
		if err != nil {
			log.Print(err)
			http.Error(w, "Không lưu được bài PDF", 500)
			return
		}
		if len(thumbnail) > 0 {
			path := filepath.Join(dir, id+".thumbnail.jpg")
			if err := os.WriteFile(path, thumbnail, 0640); err != nil {
				log.Printf("PDF thumbnail: %v", err)
			} else if store != nil {
				if err := uploadAsset(r.Context(), db, store, id, "thumbnails", path, "image/jpeg", ".jpg"); err != nil {
					log.Printf("PDF thumbnail upload will be retried: %v", err)
				}
			}
		}
		jsonResponse(w, 202, map[string]string{"recordingId": id, "jobId": jobID})
	})
}
