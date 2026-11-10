package icons

import (
	"strings"
	"testing"
)

func TestTabler(t *testing.T) {
	if Version() == "" {
		t.Fatal("no version")
	}
	if !Has("brand-docker") || Has("nope-not-an-icon") || Has("") {
		t.Fatal("Has is wrong")
	}
	svg, ok := SVG("brand-docker")
	if !ok || !strings.HasPrefix(string(svg), "<svg") || !strings.Contains(string(svg), "<path d=") {
		t.Fatalf("svg: %s", svg)
	}
	data, err := IndexJSON()
	if err != nil || !strings.Contains(string(data), `"n":"brand-docker"`) {
		t.Fatalf("index: %v", err)
	}
}
