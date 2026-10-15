package main

import "testing"

func TestYouTubeURL(t *testing.T) {
	for _, raw := range []string{"https://youtu.be/aaaS89avDVU?t=3", "https://www.youtube.com/watch?v=aaaS89avDVU&list=playlist", "https://m.youtube.com/shorts/aaaS89avDVU"} {
		got, err := youtubeURL(raw)
		if err != nil || got != "https://www.youtube.com/watch?v=aaaS89avDVU" {
			t.Fatalf("%q: %q %v", raw, got, err)
		}
	}
	for _, raw := range []string{"https://youtube.com.evil.test/watch?v=aaaS89avDVU", "https://youtube.com@localhost/watch?v=aaaS89avDVU", "http://localhost", "https://youtube.com/playlist?list=foo", "https://youtu.be/bad", "https://youtube.com:8080/watch?v=aaaS89avDVU"} {
		if _, err := youtubeURL(raw); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
}
