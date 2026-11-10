// Package config loads settings from an optional yaml file and the
// environment. Env vars win over the file, e.g. HEALTH_INTERVAL sets
// health.interval.
package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Database Database `mapstructure:"database"`
	HTTP     HTTP     `mapstructure:"http"`
	Catalog  Catalog  `mapstructure:"catalog"`
	Health   Health   `mapstructure:"health"`
	OIDC     OIDC     `mapstructure:"oidc"`
	Auth     Auth     `mapstructure:"auth"`
	Log      Log      `mapstructure:"log"`
}

type Database struct {
	URL string `mapstructure:"url"`
}

type HTTP struct {
	Addr      string `mapstructure:"addr"`
	PublicURL string `mapstructure:"public_url"`
}

type Catalog struct {
	// yaml catalog for `portal import`
	Dir string `mapstructure:"dir"`
}

type Health struct {
	Interval time.Duration `mapstructure:"interval"`
	Timeout  time.Duration `mapstructure:"timeout"`
	CAFile   string        `mapstructure:"ca_file"`
}

type OIDC struct {
	Issuer       string `mapstructure:"issuer"`
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
}

type Auth struct {
	AdminGroup string `mapstructure:"admin_group"`
	// sign in with a username and password stored in the portal
	LocalLogin bool          `mapstructure:"local_login"`
	SessionTTL time.Duration `mapstructure:"session_ttl"`
	LogoutURL  string        `mapstructure:"logout_url"`
}

type Log struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// every key needs a default, otherwise viper ignores its env var on Unmarshal
var defaults = map[string]any{
	"database.url":       "",
	"http.addr":          ":8080",
	"http.public_url":    "",
	"catalog.dir":        "",
	"health.interval":    "60s",
	"health.timeout":     "5s",
	"health.ca_file":     "",
	"oidc.issuer":        "",
	"oidc.client_id":     "",
	"oidc.client_secret": "",
	"auth.admin_group":   "",
	"auth.local_login":   false,
	"auth.session_ttl":   "12h",
	"auth.logout_url":    "",
	"log.level":          "info",
	"log.format":         "json",
}

// Load reads file (optional, yaml) and then the environment.
func Load(file string) (Config, error) {
	v := viper.New()
	for k, val := range defaults {
		v.SetDefault(k, val)
	}
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if file != "" {
		v.SetConfigFile(file)
		if err := v.ReadInConfig(); err != nil {
			return Config{}, fmt.Errorf("read config %s: %w", file, err)
		}
	}

	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	return c, nil
}

// LoginEnabled is true when oidc or local login is on.
func (c Config) LoginEnabled() bool { return c.OIDC.Issuer != "" || c.Auth.LocalLogin }

// Validate checks what `serve` needs.
func (c Config) Validate() error {
	var errs []error
	if c.Database.URL == "" {
		errs = append(errs, errors.New("database.url (DATABASE_URL) is required"))
	}
	if c.Health.Interval < 0 {
		errs = append(errs, errors.New("health.interval can't be negative, use 0 to turn checks off"))
	}
	if c.Health.Interval > 0 && c.Health.Timeout <= 0 {
		errs = append(errs, errors.New("health.timeout must be more than 0"))
	}
	if c.OIDC.Issuer != "" {
		if c.OIDC.ClientID == "" || c.OIDC.ClientSecret == "" {
			errs = append(errs, errors.New("oidc.client_id and oidc.client_secret are required when oidc.issuer is set"))
		}
		if c.HTTP.PublicURL == "" {
			errs = append(errs, errors.New("http.public_url (HTTP_PUBLIC_URL) is required when oidc.issuer is set"))
		}
	}
	if c.LoginEnabled() && c.Auth.SessionTTL <= 0 {
		errs = append(errs, errors.New("auth.session_ttl must be more than 0"))
	}
	switch c.Log.Format {
	case "json", "console":
	default:
		errs = append(errs, fmt.Errorf("log.format %q: use json or console", c.Log.Format))
	}
	return errors.Join(errs...)
}
