package catalog

import (
	"os"
	"slices"
	"strings"
	"testing"
)

func TestParseFile_Example(t *testing.T) {
	data, err := os.ReadFile("../../web/public/import-example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	entries, probs := ParseFile(data)
	if len(probs) > 0 {
		t.Fatalf("example has problems: %+v", probs)
	}
	if len(entries) != 4 {
		t.Fatalf("got %d entries", len(entries))
	}
	g := entries[0]
	if g.ID != "grafana" || g.URLs["staging"] == "" || g.Links["docs"] == "" || g.IconColor != "orange" ||
		g.Version != "v10.4.2" || !g.StartHere || g.Icon != "chart-line" {
		t.Fatalf("grafana: %+v", g)
	}
	if !slices.Equal(g.VisibleTo, []string{"everyone"}) {
		t.Fatalf("grafana visible_to: %v", g.VisibleTo)
	}
	if !slices.Equal(entries[1].VisibleTo, []string{"payments", "finance"}) {
		t.Fatalf("billing visible_to: %v", entries[1].VisibleTo)
	}
	if !slices.Equal(entries[2].VisibleTo, []string{"security"}) {
		t.Fatalf("vault visible_to: %v", entries[2].VisibleTo)
	}
	if g.Lines["urls.staging"] == 0 || g.Lines["urls.staging"] <= g.Lines["urls"] {
		t.Fatalf("lines: %v", g.Lines)
	}
}

func TestParseFile_EmptyVisibility(t *testing.T) {
	entries, probs := ParseFile([]byte("version: 1\napps:\n  - id: a\n    name: A\n    visible_to: []\n"))
	if v := entries[0].VisibleTo; len(probs) > 0 || v == nil || len(v) != 0 {
		t.Fatalf("probs %v, visible_to should be empty, not left out: %#v", probs, v)
	}
}

func TestParseFile_LeftOutVisibility(t *testing.T) {
	entries, probs := ParseFile([]byte("version: 1\napps:\n  - id: a\n    name: A\n"))
	if len(probs) > 0 || entries[0].VisibleTo != nil {
		t.Fatalf("probs %v, visible_to %#v", probs, entries[0].VisibleTo)
	}
}

func TestParseFile_Problems(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		line int
		want string
	}{
		{"not yaml", "version: 1\napps:\n\t- id: a\n", 3, "Not valid YAML"},
		{"empty", "", 1, "empty"},
		{"no version", "apps:\n  - id: a\n", 1, "Add version: 1"},
		{"bad version", "version: 2\napps:\n  - id: a\n", 1, `Version "2"`},
		{"no apps", "version: 1\n", 1, "Add an apps list"},
		{"apps not a list", "version: 1\napps: grafana\n", 2, "must be a list"},
		{"top level field", "version: 1\nservices: []\napps:\n  - id: a\n", 2, `Unknown field "services"`},
		{"app field", "version: 1\napps:\n  - id: a\n    team: x\n", 4, `Unknown field "team"`},
		{"duplicate", "version: 1\napps:\n  - id: a\n  - id: a\n", 4, "also used on line 3"},
		{"urls not a map", "version: 1\napps:\n  - id: a\n    urls: https://a\n", 4, "name: url pairs"},
		{"bad visible_to", "version: 1\napps:\n  - id: a\n    visible_to: {a: b}\n", 4, "everyone or a list"},
		{"name is a list", "version: 1\napps:\n  - id: a\n    name: [x]\n", 4, "name must be text"},
		{"start_here not a bool", "version: 1\napps:\n  - id: a\n    start_here: yes please\n", 4, "true or false"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, probs := ParseFile([]byte(tc.yaml))
			for _, p := range probs {
				if strings.Contains(p.Message, tc.want) {
					if p.Line != tc.line {
						t.Fatalf("line %d, want %d: %+v", p.Line, tc.line, p)
					}
					return
				}
			}
			t.Fatalf("no problem with %q in %+v", tc.want, probs)
		})
	}
}
