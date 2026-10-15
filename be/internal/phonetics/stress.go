package phonetics

import (
	"strings"
)

// Oxford's learner convention marks the onset of a stressed syllable, omits
// stress on monosyllables, and omits secondary stresses after primary stress.
// This changes notation only: it keeps the source's stressed vowel and accent.
// Reference: https://www.oxfordlearnersdictionaries.com/about/english/pronunciation_english
func normalizeStress(word, ipa string) string {
	if !strings.ContainsAny(ipa, "ˈˌ") {
		return ipa
	}
	if parts := strings.Fields(ipa); len(parts) > 1 {
		words := strings.Fields(word)
		for i, part := range parts {
			key := ""
			if len(words) == len(parts) {
				key = words[i]
			}
			parts[i] = normalizeStress(key, part)
		}
		return strings.Join(parts, " ")
	}
	atoms := ipaAtoms(ipa)
	nuclei := stressNuclei(word, atoms)
	if len(nuclei) == 0 {
		return ipa
	}
	strip := strings.NewReplacer("ˈ", "", "ˌ", "")
	if len(nuclei) == 1 {
		return strip.Replace(ipa)
	}
	firstPrimary := len(atoms)
	for i, atom := range atoms {
		if strings.ContainsRune(atom.prefix, 'ˈ') {
			firstPrimary = i
			break
		}
	}
	marks := make([]string, len(atoms))
	for i, atom := range atoms {
		for _, mark := range atom.prefix {
			if mark != 'ˈ' && mark != 'ˌ' {
				continue
			}
			if mark == 'ˌ' && i >= firstPrimary {
				continue
			}
			start := i
			// Already syllable-initial marks on consonants are retained. eSpeak's
			// vowel-attached marks are moved across a legal onset, never across a coda.
			if vowel(atom.text) {
				previous := -1
				for _, nucleus := range nuclei {
					if nucleus >= i {
						break
					}
					previous = nucleus
				}
				start = syllableOnset(atoms, previous+1, i)
			}
			marks[start] += string(mark)
		}
	}
	var out strings.Builder
	for i, atom := range atoms {
		out.WriteString(strip.Replace(atom.prefix))
		out.WriteString(marks[i])
		out.WriteString(atom.text)
	}
	return out.String()
}

func dictionarySyllables(word string) int {
	dictionaryOnce.Do(loadPronunciations)
	count := 0
	for _, pronunciation := range pronunciations[Key(word)] {
		candidate := 0
		for _, phone := range strings.Fields(pronunciation) {
			if len(phone) > 0 && strings.ContainsRune("012", rune(phone[len(phone)-1])) {
				candidate++
			}
		}
		if candidate > 0 && (count == 0 || candidate < count) {
			count = candidate
		}
	}
	return count
}
func stressNuclei(word string, atoms []ipaAtom) []int {
	nuclei := []int{}
	for i, atom := range atoms {
		if vowel(atom.text) || strings.ContainsRune(atom.text, '̩') {
			nuclei = append(nuclei, i)
		}
	}
	expected := dictionarySyllables(word)
	// A centring diphthong such as deer's /iə/ is one nucleus, although the
	// sound aligner keeps its two symbols separate. Do not merge hiatus in
	// words such as glorious when the lexicon has the additional syllable.
	for j := 1; j < len(nuclei) && expected > 0 && len(nuclei) > expected; {
		left, right := nuclei[j-1], nuclei[j]
		a, b := comparable(atoms[left].text), comparable(atoms[right].text)
		if right == left+1 && b == "ə" && strings.Contains("iɪeʊ", a) && len([]rune(a)) == 1 {
			nuclei = append(nuclei[:j], nuclei[j+1:]...)
		} else {
			j++
		}
	}
	// Oxford also counts a syllabic consonant, e.g. /ˈbʌtn/ or /ˈrɪðm/.
	if expected > len(nuclei) && len(atoms) > 0 {
		last := len(atoms) - 1
		if (atoms[last].text == "n" || atoms[last].text == "l" || atoms[last].text == "m") && (len(nuclei) == 0 || nuclei[len(nuclei)-1] != last) {
			nuclei = append(nuclei, last)
		}
	}
	return nuclei
}

// English syllables admit onsets such as /b/, /fr/, /str/ but not /nd/, /mp/
// or /ŋ/. Maximum legal onset keeps about /əˈbaʊt/, important /ɪmˈpɔːtənt/
// and afraid /əˈfreɪd/ distinct from simply moving every mark to word start.
func syllableOnset(atoms []ipaAtom, lower, vowelIndex int) int {
	if lower == 0 {
		return 0
	}
	start := vowelIndex
	for i := vowelIndex - 1; i >= lower; i-- {
		var cluster strings.Builder
		for j := i; j < vowelIndex; j++ {
			cluster.WriteString(comparable(atoms[j].text))
		}
		if legalOnset(cluster.String()) {
			start = i
		}
	}
	return start
}
func legalOnset(cluster string) bool {
	if len([]rune(cluster)) == 1 {
		return strings.ContainsRune("pbtdkɡgfvsʃʒθðhmnlrwj", []rune(cluster)[0])
	}
	switch cluster {
	case "pr", "br", "tr", "dr", "kr", "gr", "fr", "θr", "ʃr", "pl", "bl", "kl", "gl", "fl", "sl", "tw", "dw", "kw", "gw", "sw", "θw", "sp", "st", "sk", "spr", "str", "skr", "spl", "skw", "spj", "stj", "skj", "pj", "bj", "tj", "dj", "kj", "gj", "fj", "vj", "mj", "nj", "hj", "lj", "tʃ", "dʒ":
		return true
	}
	return false
}
