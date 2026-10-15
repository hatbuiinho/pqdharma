package core

import "testing"

func TestSample(t *testing.T) {
	v, e := Load("../../../imports/aaaS89avDVU/import.json")
	if e != nil {
		t.Fatal(e)
	}
	if len(v.Segments) != 333 {
		t.Fatalf("unexpected segments %d", len(v.Segments))
	}
	if v.Segments[0].Tokens[0].StartMs != 7040 {
		t.Fatal("word timing changed")
	}
}
func TestInvalidTiming(t *testing.T) {
	v := Transcript{Version: 1, Title: "test", Segments: []Segment{{StartMs: 10, EndMs: 30, Tokens: []Token{{Text: "word", StartMs: 20, EndMs: 40}}}}}
	if Normalize(&v) == nil {
		t.Fatal("out of segment token accepted")
	}
}
