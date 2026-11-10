package catalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const grafana = `id: grafana
name: Grafana
description: Dashboards for all services
owner: platform
contact: platform-team@yourcompany.id
category: monitoring
urls:
  prod: https://grafana.internal.yourcompany.id
links:
  repo: https://gitea.yourcompany.id/platform/grafana-config
access: Ask in the platform team channel
`

func TestLoadDir_ReadsNestedYAMLFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "grafana.yaml", grafana)
	writeFile(t, dir, "billing/billing-api.yml", "id: billing-api\nname: Billing API\n")
	writeFile(t, dir, "README.md", "not an app")

	apps, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if len(apps) != 2 {
		t.Fatalf("got %d apps, want 2", len(apps))
	}
	if apps[0].ID != "billing-api" || apps[1].ID != "grafana" {
		t.Fatalf("apps not sorted by id: %q, %q", apps[0].ID, apps[1].ID)
	}
	if apps[1].URLs["prod"] != "https://grafana.internal.yourcompany.id" {
		t.Fatalf("prod url = %q", apps[1].URLs["prod"])
	}
}

func TestValidate_RejectsBadIDs(t *testing.T) {
	for _, id := range []string{"", "Grafana", "billing_api", "-api", "api-", "a--b", "has space"} {
		err := App{ID: id, Name: "x"}.Validate()
		if err == nil {
			t.Errorf("id %q: want error, got nil", id)
		}
	}
	for _, id := range []string{"grafana", "billing-api", "k8s-dashboard2"} {
		if err := (App{ID: id, Name: "x"}).Validate(); err != nil {
			t.Errorf("id %q: unexpected error %v", id, err)
		}
	}
}

func TestValidate_RequiresNameAndHTTPURLs(t *testing.T) {
	if err := (App{ID: "a"}).Validate(); err == nil || !strings.Contains(err.Error(), "name") {
		t.Errorf("missing name: got %v", err)
	}
	bad := App{ID: "a", Name: "A", URLs: map[string]string{"prod": "grafana.internal"}}
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "urls.prod") {
		t.Errorf("relative url: got %v", err)
	}
	badLink := App{ID: "a", Name: "A", Links: map[string]string{"repo": "ftp://x"}}
	if err := badLink.Validate(); err == nil || !strings.Contains(err.Error(), "links.repo") {
		t.Errorf("ftp link: got %v", err)
	}
}

func TestLoadDir_RejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.yaml", "id: a\nname: A\nownr: typo\n")
	if _, err := LoadDir(dir); err == nil || !strings.Contains(err.Error(), "ownr") {
		t.Fatalf("want unknown-field error, got %v", err)
	}
}

func TestLoadDir_RejectsDuplicateIDsNamingBothFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "one.yaml", "id: same\nname: One\n")
	writeFile(t, dir, "two.yaml", "id: same\nname: Two\n")
	_, err := LoadDir(dir)
	if err == nil {
		t.Fatal("want duplicate error")
	}
	if !strings.Contains(err.Error(), "one.yaml") || !strings.Contains(err.Error(), "two.yaml") {
		t.Fatalf("error should name both files: %v", err)
	}
}

func TestLoadDir_ReportsEveryBadFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.yaml", "id: A\nname: A\n")
	writeFile(t, dir, "b.yaml", "id: b\n")
	_, err := LoadDir(dir)
	if err == nil || !strings.Contains(err.Error(), "a.yaml") || !strings.Contains(err.Error(), "b.yaml") {
		t.Fatalf("want errors for both files, got %v", err)
	}
}

func TestValidate_AllowedGroups(t *testing.T) {
	ok := App{ID: "a", Name: "A", AllowedGroups: []string{"platform", "admins"}}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := App{ID: "a", Name: "A", AllowedGroups: []string{"platform", " "}}
	if err := bad.Validate(); err == nil {
		t.Fatal("want error for empty group")
	}
}

func TestHealthTarget(t *testing.T) {
	cases := []struct {
		name string
		app  App
		want string
	}{
		{"no urls", App{}, ""},
		{"prod url", App{URLs: map[string]string{"staging": "https://s.example", "prod": "https://p.example"}}, "https://p.example"},
		{"first url by name without prod", App{URLs: map[string]string{"staging": "https://s.example", "dev": "https://d.example"}}, "https://d.example"},
		{"path joined to prod", App{HealthURL: "/healthz", URLs: map[string]string{"prod": "https://p.example/app/"}}, "https://p.example/healthz"},
		{"absolute health url", App{HealthURL: "http://10.0.0.5:9000/ready", URLs: map[string]string{"prod": "https://p.example"}}, "http://10.0.0.5:9000/ready"},
		{"path without any url", App{HealthURL: "/healthz"}, ""},
	}
	for _, c := range cases {
		if got := c.app.HealthTarget(); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
