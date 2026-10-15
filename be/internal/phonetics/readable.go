package phonetics

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"dharma-player/internal/core"
	_ "embed"
	"strings"
	"sync"
	"unicode"
)

// CMUdict is bundled locally; there are no requests or extra synthesis runs.
// Attribution, pinned source and redistribution license are in data/.
//
//go:embed data/cmudict.dict.gz
var dictionaryData []byte
var dictionaryOnce sync.Once
var pronunciations map[string][]string

func loadPronunciations() {
	pronunciations = map[string][]string{}
	reader, err := gzip.NewReader(bytes.NewReader(dictionaryData))
	if err != nil {
		panic(err)
	}
	defer reader.Close()
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := strings.SplitN(scanner.Text(), " #", 2)[0]
		word, phones, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		if i := strings.IndexByte(word, '('); i >= 0 {
			word = word[:i]
		}
		pronunciations[word] = append(pronunciations[word], phones)
	}
	if err := scanner.Err(); err != nil {
		panic(err)
	}
}

var arpabet = map[string]string{
	"AA": "ɑ", "AE": "æ", "AH": "ʌ", "AO": "ɔ", "AW": "aʊ", "AY": "aɪ", "B": "b", "CH": "tʃ", "D": "d", "DH": "ð", "EH": "e", "ER": "ɚ", "EY": "eɪ", "F": "f", "G": "ɡ", "HH": "h", "IH": "ɪ", "IY": "i", "JH": "dʒ", "K": "k", "L": "l", "M": "m", "N": "n", "NG": "ŋ", "OW": "oʊ", "OY": "ɔɪ", "P": "p", "R": "r", "S": "s", "SH": "ʃ", "T": "t", "TH": "θ", "UH": "ʊ", "UW": "u", "V": "v", "W": "w", "Y": "j", "Z": "z", "ZH": "ʒ",
}

func referencePhones(pronunciation string) []string {
	out := []string{}
	for _, phone := range strings.Fields(pronunciation) {
		symbol := arpabet[strings.TrimRight(phone, "012")]
		if phone == "AH0" {
			symbol = "ə"
		}
		if symbol != "" {
			out = append(out, symbol)
		}
	}
	return out
}

type ipaAtom struct{ prefix, text string }

func ipaAtoms(ipa string) []ipaAtom {
	runes := []rune(ipa)
	out := []ipaAtom{}
	prefix := ""
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == 'ˈ' || r == 'ˌ' || unicode.IsSpace(r) {
			prefix += string(r)
			continue
		}
		atom := string(r)
		if i+1 < len(runes) {
			pair := string(runes[i : i+2])
			switch pair {
			case "aɪ", "aʊ", "eɪ", "oʊ", "əʊ", "ɔɪ", "tʃ", "dʒ":
				atom = pair
				i++
			}
		}
		for i+1 < len(runes) && (runes[i+1] == 'ː' || runes[i+1] == 'ˑ' || unicode.Is(unicode.Mn, runes[i+1])) {
			i++
			atom += string(runes[i])
		}
		out = append(out, ipaAtom{prefix, atom})
		prefix = ""
	}
	if prefix != "" {
		out = append(out, ipaAtom{prefix, ""})
	}
	return out
}
func comparable(phone string) string {
	phone = strings.Map(func(r rune) rune {
		if r == 'ː' || r == 'ˑ' || unicode.Is(unicode.Mn, r) {
			return -1
		}
		return r
	}, phone)
	return strings.NewReplacer("ɹ", "r", "ɛ", "e", "ɡ", "g").Replace(phone)
}
func vowel(phone string) bool { return strings.ContainsAny(phone, "aeiouɑæʌɔɪɚɜəʊɒᵻɐ") }
func phoneCost(raw, reference string) int {
	a, b := comparable(raw), comparable(reference)
	if a == b {
		return 0
	}
	if a == "ᵻ" && (b == "ɪ" || b == "ə") {
		return 0
	}
	if a == "ɐ" && b == "ə" {
		return 0
	}
	if a == "ɾ" && (b == "t" || b == "d") {
		return 0
	}
	if vowel(a) && vowel(b) {
		return 1
	} // Permit accent differences; never copy their other vowels.
	if vowel(a) != vowel(b) {
		return 8
	}
	return 5
}

// Align complete sound sequences, so editor's two taps become d then t, rather
// than using a global replacement or guessing from the first spelling letter.
func alignPhones(raw []ipaAtom, reference []string) (int, []int) {
	n, m := len(raw), len(reference)
	if n == 0 || m == 0 || n > 96 || m > 96 {
		return 1000, nil
	}
	costs := make([][]int, n+1)
	steps := make([][]byte, n+1)
	for i := range costs {
		costs[i] = make([]int, m+1)
		steps[i] = make([]byte, m+1)
		costs[i][0] = 3 * i
	}
	for j := 0; j <= m; j++ {
		costs[0][j] = 3 * j
	}
	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			costs[i][j] = costs[i-1][j-1] + phoneCost(raw[i-1].text, reference[j-1])
			steps[i][j] = 'm'
			if deletion := costs[i-1][j] + 3; deletion < costs[i][j] {
				costs[i][j] = deletion
				steps[i][j] = 'd'
			}
			if insertion := costs[i][j-1] + 3; insertion < costs[i][j] {
				costs[i][j] = insertion
				steps[i][j] = 'i'
			}
		}
	}
	matched := make([]int, n)
	for i := range matched {
		matched[i] = -1
	}
	for i, j := n, m; i > 0 || j > 0; {
		if i == 0 {
			j--
			continue
		}
		if j == 0 {
			i--
			continue
		}
		switch steps[i][j] {
		case 'm':
			matched[i-1] = j - 1
			i--
			j--
		case 'd':
			i--
		default:
			j--
		}
	}
	return costs[n][m], matched
}

// ReadableIPA is a display convention, not a second generated accent. Original
// phonetics and reviewed database values are never rewritten.
func ReadableIPA(word string, p core.Phonetic, voice string) string {
	if p.Type != "ipa" {
		return p.Text
	}
	words, utterances := strings.Fields(word), strings.Fields(p.Text)
	if len(words) > 1 && len(words) == len(utterances) {
		for i, part := range utterances {
			copy := p
			copy.Text = part
			utterances[i] = ReadableIPA(words[i], copy, voice)
		}
		return strings.Join(utterances, " ")
	}
	key := Key(word)
	if key == "meditative" && (p.Source == "generated" || p.Source == "") {
		if voice == "en-gb" {
			return "ˈmedɪtətɪv"
		}
		if voice == "en-us" || strings.Contains(p.Text, "eɪ") {
			return "ˈmedɪteɪtɪv"
		}
		return "ˈmedɪtətɪv"
	}
	atoms := ipaAtoms(p.Text)
	bestCost := 1000
	var matched []int
	var reference []string
	if strings.ContainsAny(p.Text, "ᵻɾ") {
		dictionaryOnce.Do(loadPronunciations)
		for _, pronunciation := range pronunciations[key] {
			phones := referencePhones(pronunciation)
			cost, alignment := alignPhones(atoms, phones)
			if cost < bestCost {
				bestCost = cost
				matched = alignment
				reference = phones
			}
		}
	}
	// Reject poorly matching entries instead of borrowing sounds from an unrelated pronunciation.
	reliable := matched != nil && bestCost <= max(3, len(atoms)/3)
	var out strings.Builder
	for i, atom := range atoms {
		text := atom.text
		ref := ""
		if reliable && matched[i] >= 0 {
			ref = reference[matched[i]]
		}
		switch text {
		case "ᵻ":
			text = "ɪ"
			if ref == "ə" {
				text = "ə"
			}
		case "ɐ":
			text = "ə" // eSpeak's unstressed open-schwa variant.
		case "ɾ":
			if ref == "t" || ref == "d" {
				text = ref
			} else if p.Source == "generated" {
				text = "t"
			} // eSpeak en-us t#.
		}
		text = strings.NewReplacer("ɹ", "r", "ɛ", "e").Replace(text)
		out.WriteString(atom.prefix)
		out.WriteString(text)
	}
	return normalizeStress(word, out.String())
}

// PrepareReadable returns an independent response. Internal jobs and database
// reads keep the original objects, so this cannot change persisted phonetics.
func PrepareReadable(t core.Transcript) core.Transcript {
	out := t
	out.Segments = append([]core.Segment(nil), t.Segments...)
	type cacheKey struct{ word, raw, voice, source string }
	cache := map[cacheKey]string{}
	prepare := func(word, voice string, p *core.Phonetic) *core.Phonetic {
		if p == nil {
			return nil
		}
		copy := *p
		if p.Type == "ipa" {
			key := cacheKey{word, p.Text, voice, p.Source}
			text, ok := cache[key]
			if !ok {
				text = ReadableIPA(word, *p, voice)
				cache[key] = text
			}
			copy.Readable = text
		}
		return &copy
	}
	for si := range out.Segments {
		out.Segments[si].Tokens = append([]core.Token(nil), t.Segments[si].Tokens...)
		for ti := range out.Segments[si].Tokens {
			token := &out.Segments[si].Tokens[ti]
			token.Phonetic = prepare(token.Text, t.DefaultIPAVoice, token.Phonetic)
			if token.Phonetics != nil {
				variants := map[string]*core.Phonetic{}
				for voice, p := range token.Phonetics {
					variants[voice] = prepare(token.Text, voice, p)
				}
				token.Phonetics = variants
			}
		}
	}
	return out
}
