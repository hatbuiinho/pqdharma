package main

import (
	"strings"
	"testing"
)

func TestParsePDFDialogueAcrossPages(t *testing.T) {
	turns := parsePDFTurns("Woman: Hello.\nMan:\u200b I would like\nsome information.\fWoman: Here it is.\n")
	if len(turns) != 3 || turns[1].Speaker != "man" || turns[1].Text != "I would like some information." || turns[2].Text != "Here it is." {
		t.Fatalf("unexpected turns: %+v", turns)
	}
	if len(parsePDFTurns("\n\f \u200b")) != 0 {
		t.Fatal("empty/scanned document should not create speech")
	}
	plain := parsePDFTurns("A simple text\ncontinues here.")
	if len(plain) != 1 || plain[0].Speaker != "narrator" || plain[0].Text != "A simple text continues here." {
		t.Fatalf("plain text: %+v", plain)
	}
}

func TestValidatePDFSpeech(t *testing.T) {
	valid := func() pdfImport {
		return pdfImport{Title: "Example", Turns: []pdfTurn{{Speaker: "woman", Text: "Hello."}}, Voices: map[string]string{"woman": "bf_emma"}, Speed: 1, Gap: .5}
	}
	input := valid()
	if err := validatePDFImport(&input); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*pdfImport){
		func(p *pdfImport) { p.Turns = nil },
		func(p *pdfImport) { p.Turns[0].Text = " " },
		func(p *pdfImport) { p.Turns[0].Text = strings.Repeat("x", 4001) },
		func(p *pdfImport) { p.Turns[0].Speaker = "unknown" },
		func(p *pdfImport) { p.Voices["woman"] = "unknown" },
		func(p *pdfImport) { p.Speed = 0 },
		func(p *pdfImport) { p.Gap = 3 },
	} {
		p := valid()
		change(&p)
		if validatePDFImport(&p) == nil {
			t.Fatalf("accepted invalid request: %+v", p)
		}
	}
}

func TestParseLongPDFProseWithoutLosingWords(t *testing.T) {
	text := strings.Repeat("A long sentence. ", 700)
	turns := parsePDFTurns(text)
	joined := []string{}
	for _, turn := range turns {
		if len([]rune(turn.Text)) > 3000 || turn.Speaker != "narrator" {
			t.Fatalf("invalid chunk: %+v", turn)
		}
		joined = append(joined, turn.Text)
	}
	if strings.Join(joined, " ") != strings.TrimSpace(text) {
		t.Fatal("chunking changed the text")
	}
}
