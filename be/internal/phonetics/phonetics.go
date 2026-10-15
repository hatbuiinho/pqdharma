// Package phonetics enriches subtitle tokens without changing text or timing.
package phonetics

import (
	"context"
	"dharma-player/internal/core"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode"
)

type Generator struct {
	Voice     string
	Overrides map[string]string
	// Run may be replaced by tests. Text is passed through stdin, never a shell.
	Run        func(context.Context, string, string) (string, error)
	OnProgress func(done, total int) error
}

func LoadOverrides(path string) (map[string]string, error) {
	out := map[string]string{}
	if path == "" {
		return out, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	normalized := map[string]string{}
	for key, value := range out {
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("empty IPA override for %q", key)
		}
		normalized[Key(key)] = strings.TrimSpace(value)
	}
	return normalized, nil
}

func Key(text string) string {
	return strings.TrimFunc(strings.ToLower(strings.TrimSpace(text)), func(r rune) bool { return unicode.IsPunct(r) || unicode.IsSpace(r) })
}

func Espeak(ctx context.Context, voice, text string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "espeak-ng", "-q", "--ipa", "-v", voice, "--stdin")
	cmd.Stdin = strings.NewReader(text + "\n")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("espeak-ng: %w: %s", err, stderr.String())
	}
	ipa := strings.Join(strings.Fields(string(out)), " ")
	if ipa == "" {
		return "", fmt.Errorf("espeak-ng returned empty IPA for %q", text)
	}
	return ipa, nil
}

// Enrich computes everything first, so a failure leaves the transcript untouched.
// Existing phonetics, including editorial corrections, always take precedence.
func (g Generator) Enrich(ctx context.Context, t *core.Transcript) (int, error) {
	if t.Language != "en" && !strings.HasPrefix(t.Language, "en-") {
		return 0, nil
	}
	if g.Voice != "en-us" && g.Voice != "en-gb" {
		return 0, fmt.Errorf("unsupported IPA voice %q", g.Voice)
	}
	run := g.Run
	if run == nil {
		run = Espeak
	}
	cache := map[string]string{}
	type update struct {
		token  *core.Token
		text   string
		source string
	}
	total := 0
	for _, segment := range t.Segments {
		for _, token := range segment.Tokens {
			if needsIPA(token) {
				total++
			}
		}
	}
	if g.OnProgress != nil {
		if err := g.OnProgress(0, total); err != nil {
			return 0, err
		}
	}
	pending := []update{}
	for si := range t.Segments {
		for ti := range t.Segments[si].Tokens {
			token := &t.Segments[si].Tokens[ti]
			if !needsIPA(*token) {
				continue
			}
			key := Key(token.Text)
			ipa, ok := g.Overrides[key]
			source := "generated"
			if ok {
				source = "override"
			}
			if !ok {
				ipa, ok = cache[token.Text]
			}
			if !ok {
				var err error
				ipa, err = run(ctx, g.Voice, token.Text)
				if err != nil {
					return 0, fmt.Errorf("token %s (%q): %w", token.ID, token.Text, err)
				}
				cache[token.Text] = ipa
			}
			pending = append(pending, update{token, ipa, source})
			if g.OnProgress != nil {
				if err := g.OnProgress(len(pending), total); err != nil {
					return 0, err
				}
			}
		}
	}
	for _, item := range pending {
		item.token.Phonetic = &core.Phonetic{Type: "ipa", Text: item.text, Source: item.source}
	}
	return len(pending), nil
}

func needsIPA(token core.Token) bool {
	return token.Phonetic == nil && strings.ContainsFunc(Key(token.Text), func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) })
}
