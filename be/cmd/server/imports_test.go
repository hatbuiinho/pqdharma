package main

import (
	"dharma-player/internal/core"
	"testing"
)

func TestEditingPreservesOnlyUnchangedIPA(t *testing.T) {
	reviewed := &core.Phonetic{Type: "ipa", Text: "reviewed"}
	old := []core.Token{{Text: "Thus", Phonetic: reviewed}, {Text: "heard", Phonetic: reviewed}}
	updated := []core.Token{{Text: "Thus"}, {Text: "hear"}, {Text: "now"}}
	preserveIPA(old, updated)
	if updated[0].Phonetic != reviewed || updated[1].Phonetic != nil || updated[2].Phonetic != nil {
		t.Fatal("reviewed pronunciation overwritten or copied to changed text")
	}
}
