package main

import (
	"strings"
	"testing"
)

func TestShareHTML(t *testing.T) {
	output := shareHTML(`<head><title>Old</title><meta name="description" content="Old"><script src="/app.js"></script></head><body>App</body>`, `Title " & <script>`, `Description & text`, "https://example.com/recordings/test", "https://example.com/image.jpg")
	for _, expected := range []string{`property="og:title"`, `name="twitter:card"`, `rel="canonical"`, `Title &#34; &amp; &lt;script&gt;`, `<script src="/app.js"></script>`, `<body>App</body>`} {
		if !strings.Contains(output, expected) {
			t.Fatalf("missing %s", expected)
		}
	}
	if strings.Count(output, "<title>") != 1 || strings.Count(output, `name="description"`) != 1 || strings.Contains(output, "Old") {
		t.Fatal("stale metadata")
	}
}
