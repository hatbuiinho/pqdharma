package core

import (
	"encoding/json"
	"fmt"
	"os"
)

type Phonetic struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Source   string `json:"source,omitempty"`
	Readable string `json:"readable,omitempty"`
}
type Token struct {
	ID           string               `json:"id"`
	Text         string               `json:"text"`
	StartMs      int                  `json:"startMs"`
	EndMs        int                  `json:"endMs"`
	EndEstimated bool                 `json:"endEstimated"`
	Phonetic     *Phonetic            `json:"phonetic,omitempty"`
	Phonetics    map[string]*Phonetic `json:"phonetics,omitempty"`
}
type Segment struct {
	ID      string  `json:"id"`
	StartMs int     `json:"startMs"`
	EndMs   int     `json:"endMs"`
	Text    string  `json:"text"`
	Tokens  []Token `json:"tokens"`
	Words   []Token `json:"words,omitempty"`
}
type Recording struct {
	Title     string `json:"title"`
	AudioFile string `json:"audioFile"`
}
type Transcript struct {
	DefaultIPAVoice string    `json:"defaultIpaVoice,omitempty"`
	IPAVoices       []string  `json:"ipaVoices"`
	IPAEditable     bool      `json:"ipaEditable"`
	Version         int       `json:"version"`
	Revision        int       `json:"revision"`
	Title           string    `json:"title"`
	Language        string    `json:"language"`
	SourceURL       string    `json:"sourceUrl"`
	Status          string    `json:"status"`
	Recording       Recording `json:"recording"`
	Segments        []Segment `json:"segments"`
}

func Load(path string) (Transcript, error) {
	var t Transcript
	b, e := os.ReadFile(path)
	if e != nil {
		return t, e
	}
	if e = json.Unmarshal(b, &t); e != nil {
		return t, e
	}
	e = Normalize(&t)
	return t, e
}
func Normalize(t *Transcript) error {
	if t.Version != 1 || t.Title == "" || len(t.Segments) == 0 {
		return fmt.Errorf("version, title and segments required")
	}
	lastEnd := 0
	for i := range t.Segments {
		s := &t.Segments[i]
		if len(s.Tokens) == 0 {
			s.Tokens = s.Words
		}
		s.Words = nil
		s.ID = fmt.Sprintf("segment-%04d", i+1)
		if s.StartMs < lastEnd || s.EndMs <= s.StartMs || len(s.Tokens) == 0 {
			return fmt.Errorf("invalid segment %d", i+1)
		}
		previous := s.StartMs
		for j := range s.Tokens {
			w := &s.Tokens[j]
			w.ID = fmt.Sprintf("%s-token-%04d", s.ID, j+1)
			if w.Text == "" || w.StartMs < previous || w.EndMs <= w.StartMs || w.EndMs > s.EndMs {
				return fmt.Errorf("invalid token %d/%d", i+1, j+1)
			}
			previous = w.EndMs
		}
		lastEnd = s.EndMs
	}
	return nil
}
