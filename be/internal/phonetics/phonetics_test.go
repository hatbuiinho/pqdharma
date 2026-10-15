package phonetics

import (
	"context"
	"dharma-player/internal/core"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestEnrichPreservesTimingAndReviewedIPA(t *testing.T) {
	reviewed := &core.Phonetic{Type: "ipa", Text: "reviewed"}
	transcript := core.Transcript{Language: "en", Segments: []core.Segment{{Tokens: []core.Token{
		{ID: "a", Text: "hello", StartMs: 10, EndMs: 20, EndEstimated: true},
		{ID: "b", Text: "hello", StartMs: 20, EndMs: 30},
		{ID: "c", Text: "Buddha,", StartMs: 30, EndMs: 40},
		{ID: "d", Text: "read", StartMs: 40, EndMs: 50, Phonetic: reviewed},
		{ID: "e", Text: "right effort", StartMs: 50, EndMs: 70},
		{ID: "f", Text: "...", StartMs: 70, EndMs: 80},
	}}}}
	before := append([]core.Token(nil), transcript.Segments[0].Tokens...)
	calls := 0
	generator := Generator{Voice: "en-us", Overrides: map[string]string{"buddha": "ˈbʊdə"}, Run: func(_ context.Context, voice, text string) (string, error) { calls++; return "IPA " + text, nil }}
	count, err := generator.Enrich(context.Background(), &transcript)
	if err != nil || count != 4 || calls != 2 {
		t.Fatalf("count=%d calls=%d error=%v", count, calls, err)
	}
	tokens := transcript.Segments[0].Tokens
	if tokens[2].Phonetic.Text != "ˈbʊdə" || tokens[3].Phonetic != reviewed || tokens[4].Phonetic.Text != "IPA right effort" || tokens[5].Phonetic != nil {
		t.Fatal("override, editorial or phrase handling changed")
	}
	for i, token := range tokens {
		token.Phonetic = before[i].Phonetic
		if !reflect.DeepEqual(token, before[i]) {
			t.Fatal("token text or timing changed")
		}
	}
	count, err = generator.Enrich(context.Background(), &transcript)
	if err != nil || count != 0 || calls != 2 {
		t.Fatal("enrichment is not idempotent")
	}
}

func TestFailureIsAtomic(t *testing.T) {
	transcript := core.Transcript{Language: "en", Segments: []core.Segment{{Tokens: []core.Token{{Text: "first"}, {Text: "second"}}}}}
	generator := Generator{Voice: "en-us", Run: func(_ context.Context, _, text string) (string, error) {
		if text == "second" {
			return "", errors.New("unavailable")
		}
		return "IPA", nil
	}}
	if _, err := generator.Enrich(context.Background(), &transcript); err == nil {
		t.Fatal("failure swallowed")
	}
	for _, token := range transcript.Segments[0].Tokens {
		if token.Phonetic != nil {
			t.Fatal("partial update after failure")
		}
	}
}

func TestNonEnglishUntouched(t *testing.T) {
	transcript := core.Transcript{Language: "vi"}
	count, err := (Generator{}).Enrich(context.Background(), &transcript)
	if err != nil || count != 0 {
		t.Fatal("non-English transcript enriched")
	}
}

// A tiny executable verifies the real process boundary without host eSpeak.
func TestEspeakStdinIncludesFinalNewline(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\nIFS= read -r text || exit 1\n[ \"$text\" = I ] || exit 2\nprintf '  ˈaɪ  \\n'\n"
	if err := os.WriteFile(filepath.Join(dir, "espeak-ng"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	ipa, err := Espeak(context.Background(), "en-us", "I")
	if err != nil || ipa != "ˈaɪ" {
		t.Fatalf("IPA=%q error=%v", ipa, err)
	}
}

func TestProgressCountsMissingWordsAndPreservesAtomicFailure(t *testing.T) {
	makeTranscript := func() core.Transcript {
		return core.Transcript{Language: "en", Segments: []core.Segment{{Tokens: []core.Token{
			{Text: "same"}, {Text: "same"}, {Text: "..."}, {Text: "reviewed", Phonetic: &core.Phonetic{Type: "ipa", Text: "manual"}},
		}}}}
	}
	transcript := makeTranscript()
	events := [][2]int{}
	generator := Generator{Voice: "en-us", Run: func(context.Context, string, string) (string, error) { return "ipa", nil }, OnProgress: func(done, total int) error { events = append(events, [2]int{done, total}); return nil }}
	if count, err := generator.Enrich(context.Background(), &transcript); err != nil || count != 2 {
		t.Fatalf("count=%d error=%v", count, err)
	}
	if !reflect.DeepEqual(events, [][2]int{{0, 2}, {1, 2}, {2, 2}}) {
		t.Fatalf("unexpected progress: %v", events)
	}
	transcript = makeTranscript()
	generator.OnProgress = func(done, total int) error {
		if done == 2 {
			return errors.New("progress persistence failed")
		}
		return nil
	}
	if _, err := generator.Enrich(context.Background(), &transcript); err == nil {
		t.Fatal("callback failure ignored")
	}
	if transcript.Segments[0].Tokens[0].Phonetic != nil || transcript.Segments[0].Tokens[1].Phonetic != nil {
		t.Fatal("callback failure published partial IPA")
	}
}
