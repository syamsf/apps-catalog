package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaults(t *testing.T) {
	c, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if c.HTTP.Addr != ":8080" || c.Health.Interval != time.Minute || c.Auth.SessionTTL != 12*time.Hour || c.Log.Format != "json" {
		t.Fatalf("defaults: %+v", c)
	}
	if c.LoginEnabled() {
		t.Fatal("login should be off by default")
	}
}

func TestFileThenEnv(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.yaml")
	err := os.WriteFile(file, []byte(`
database:
  url: postgres://from-file
health:
  interval: 30s
oidc:
  issuer: https://auth.example
  client_id: portal
`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEALTH_INTERVAL", "10s")
	t.Setenv("OIDC_CLIENT_SECRET", "from-env")
	t.Setenv("AUTH_ADMIN_GROUP", "admins")

	c, err := Load(file)
	if err != nil {
		t.Fatal(err)
	}
	if c.Database.URL != "postgres://from-file" {
		t.Errorf("database.url = %q", c.Database.URL)
	}
	if c.Health.Interval != 10*time.Second {
		t.Errorf("env should win over file, health.interval = %v", c.Health.Interval)
	}
	if c.OIDC.ClientID != "portal" || c.OIDC.ClientSecret != "from-env" || c.Auth.AdminGroup != "admins" {
		t.Errorf("oidc/auth: %+v %+v", c.OIDC, c.Auth)
	}
}

func TestMissingFile(t *testing.T) {
	if _, err := Load("/does/not/exist.yaml"); err == nil {
		t.Fatal("want error")
	}
}

func TestBadDuration(t *testing.T) {
	t.Setenv("HEALTH_TIMEOUT", "soon")
	if _, err := Load(""); err == nil {
		t.Fatal("want parse error")
	}
}

func TestValidate(t *testing.T) {
	c, _ := Load("")
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("want database error, got %v", err)
	}

	c.Database.URL = "postgres://x"
	if err := c.Validate(); err != nil {
		t.Fatalf("minimal config: %v", err)
	}

	c.OIDC.Issuer = "https://auth.example"
	err = c.Validate()
	if err == nil || !strings.Contains(err.Error(), "client_id") || !strings.Contains(err.Error(), "public_url") {
		t.Fatalf("want oidc errors, got %v", err)
	}

	c.OIDC = OIDC{}
	c.Auth.LocalLogin = true
	if err := c.Validate(); err != nil || !c.LoginEnabled() {
		t.Fatalf("local login only needs no oidc settings: %v", err)
	}
	c.Auth.LocalLogin = false

	c.Log.Format = "pretty"
	if err := c.Validate(); err == nil {
		t.Fatal("want log format error")
	}
}
