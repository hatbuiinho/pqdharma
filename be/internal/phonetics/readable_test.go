package phonetics

import (
	"dharma-player/internal/core"
	"reflect"
	"testing"
)

func TestReadableIPAResolvesWordsAndPreservesAccent(t *testing.T) {
	cases := []struct{ word, raw, voice, source, want string }{
		{"meditative", "mˈɛdᵻtˌeɪɾɪv", "en-us", "generated", "ˈmedɪteɪtɪv"},
		{"Meditative,", "mˈɛdɪtətˌɪv", "en-gb", "generated", "ˈmedɪtətɪv"},
		{"meditative", "mˈɛdᵻtˌeɪɾɪv", "", "", "ˈmedɪteɪtɪv"},
		{"middle city", "mˈɪɾəl sˈɪɾi", "en-us", "manual", "ˈmɪdəl ˈsɪti"},
		{"editor", "ˈɛɾɪɾɚ", "en-us", "manual", "ˈedɪtɚ"},
		{"city", "sˈɪɾi", "en-us", "generated", "ˈsɪti"},
		{"ladder", "lˈæɾɚ", "en-us", "manual", "ˈlædɚ"},
		{"water", "wˈɔːɾɚ", "en-us", "generated", "ˈwɔːtɚ"},
		{"possible", "pˈɑːsᵻbəl", "en-us", "generated", "ˈpɑːsəbəl"},
		{"roses", "ɹˈoʊzᵻz", "en-us", "generated", "ˈroʊzɪz"},
		{"wanted", "wˈɔntᵻd", "en-us", "generated", "ˈwɔntɪd"},
		{"about", "ɐbˈaʊt", "en-gb", "generated", "əˈbaʊt"},
		{"castle", "kˈɑːsəl", "en-gb", "generated", "ˈkɑːsəl"},
		{"right", "ɹˈaɪt", "en-gb", "generated", "raɪt"},
		{"madeupnamezz", "ɾᵻɐ", "en-us", "manual", "ɾɪə"},
		{"madeupnamezz", "ɾᵻɐ", "en-us", "generated", "tɪə"},
		{"meditative", "custom", "en-gb", "manual", "custom"},
	}
	for _, c := range cases {
		t.Run(c.word+"-"+c.voice+"-"+c.source, func(t *testing.T) {
			p := core.Phonetic{Type: "ipa", Text: c.raw, Source: c.source}
			if got := ReadableIPA(c.word, p, c.voice); got != c.want {
				t.Fatalf("%q => %q, want %q", c.raw, got, c.want)
			}
			if p.Text != c.raw {
				t.Fatal("raw IPA changed")
			}
		})
	}
	if got := ReadableIPA("anything", core.Phonetic{Type: "custom", Text: "ɾᵻɐ"}, "en-us"); got != "ɾᵻɐ" {
		t.Fatal("non-IPA changed")
	}
}

func TestPrepareReadableDoesNotMutatePlaybackOrStoredPhonetics(t *testing.T) {
	gb := &core.Phonetic{Type: "ipa", Text: "mˈɛdɪtətˌɪv", Source: "generated"}
	us := &core.Phonetic{Type: "ipa", Text: "mˈɛdᵻtˌeɪɾɪv", Source: "generated"}
	original := core.Transcript{Revision: 4, DefaultIPAVoice: "en-gb", Segments: []core.Segment{{ID: "s", StartMs: 10, EndMs: 50, Tokens: []core.Token{{ID: "w", Text: "meditative", StartMs: 10, EndMs: 50, Phonetic: gb, Phonetics: map[string]*core.Phonetic{"en-gb": gb, "en-us": us}}}}}}
	prepared := PrepareReadable(original)
	token := prepared.Segments[0].Tokens[0]
	if token.Phonetic.Readable != "ˈmedɪtətɪv" || token.Phonetics["en-us"].Readable != "ˈmedɪteɪtɪv" {
		t.Fatal("accent versions lost")
	}
	if gb.Readable != "" || us.Readable != "" || original.Segments[0].Tokens[0].Phonetics["en-us"] != us {
		t.Fatal("source objects mutated")
	}
	token.Phonetic = original.Segments[0].Tokens[0].Phonetic
	token.Phonetics = original.Segments[0].Tokens[0].Phonetics
	if !reflect.DeepEqual(token, original.Segments[0].Tokens[0]) || prepared.Revision != original.Revision || prepared.DefaultIPAVoice != original.DefaultIPAVoice {
		t.Fatal("display conversion changed words, times, identity or default")
	}
	prepared.Segments[0].Tokens[0].Text = "changed"
	if original.Segments[0].Tokens[0].Text != "meditative" {
		t.Fatal("response shares mutable token slice")
	}
}
