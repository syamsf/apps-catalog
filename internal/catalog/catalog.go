// Package catalog loads the app YAML files from the catalog repo.
package catalog

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// App is one entry in the catalog.
type App struct {
	ID          string            `yaml:"id" json:"id"`
	Name        string            `yaml:"name" json:"name"`
	Description string            `yaml:"description" json:"description"`
	Owner       string            `yaml:"owner" json:"owner"`
	Contact     string            `yaml:"contact" json:"contact"`
	Category    string            `yaml:"category" json:"category"`
	Access      string            `yaml:"access" json:"access"`
	HealthURL   string            `yaml:"health_url" json:"health_url"`
	URLs        map[string]string `yaml:"urls" json:"urls"`
	Links       map[string]string `yaml:"links" json:"links"`

	// empty means everyone can see the app
	AllowedGroups []string `yaml:"allowed_groups" json:"allowed_groups"`
}

var idPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func ValidID(id string) bool { return idPattern.MatchString(id) }

func (a App) Validate() error {
	var errs []error
	if !ValidID(a.ID) {
		errs = append(errs, fmt.Errorf("id %q must be lowercase letters, numbers and single dashes", a.ID))
	}
	if strings.TrimSpace(a.Name) == "" {
		errs = append(errs, errors.New("name is required"))
	}
	if a.HealthURL != "" && !isHTTPURL(a.HealthURL) && !strings.HasPrefix(a.HealthURL, "/") {
		errs = append(errs, fmt.Errorf("health_url %q must be an http(s) URL or a path like /healthz", a.HealthURL))
	}
	for _, k := range sortedKeys(a.URLs) {
		if !isHTTPURL(a.URLs[k]) {
			errs = append(errs, fmt.Errorf("urls.%s %q must be an http(s) URL", k, a.URLs[k]))
		}
	}
	for _, g := range a.AllowedGroups {
		if strings.TrimSpace(g) == "" {
			errs = append(errs, errors.New("allowed_groups has an empty entry"))
		}
	}
	for _, k := range sortedKeys(a.Links) {
		if !isHTTPURL(a.Links[k]) {
			errs = append(errs, fmt.Errorf("links.%s %q must be an http(s) URL", k, a.Links[k]))
		}
	}
	return errors.Join(errs...)
}

// PrimaryURL is the prod url, or the first one by name.
func (a App) PrimaryURL() string {
	if u, ok := a.URLs["prod"]; ok {
		return u
	}
	if keys := sortedKeys(a.URLs); len(keys) > 0 {
		return a.URLs[keys[0]]
	}
	return ""
}

// HealthTarget is the url the health checker should hit. A health_url
// path like /healthz is joined to the primary url.
func (a App) HealthTarget() string {
	if isHTTPURL(a.HealthURL) {
		return a.HealthURL
	}
	base := a.PrimaryURL()
	if base == "" || a.HealthURL == "" {
		return base
	}
	u, err := url.Parse(base)
	if err != nil {
		return base
	}
	ref, err := url.Parse(a.HealthURL)
	if err != nil {
		return base
	}
	return u.ResolveReference(ref).String()
}

// LoadDir loads all yaml files under dir, one app per file.
// Returns every error found, not only the first one.
func LoadDir(dir string) ([]App, error) {
	var (
		apps []App
		errs []error
		seen = map[string]string{} // id -> file
	)
	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != dir && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir // skip .git etc
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		app, err := loadFile(path)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", rel, err))
			return nil
		}
		if prev, dup := seen[app.ID]; dup {
			errs = append(errs, fmt.Errorf("%s: id %q is already used by %s", rel, app.ID, prev))
			return nil
		}
		seen[app.ID] = rel
		apps = append(apps, app)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	slices.SortFunc(apps, func(a, b App) int { return strings.Compare(a.ID, b.ID) })
	return apps, nil
}

func loadFile(path string) (App, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return App{}, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	var app App
	if err := dec.Decode(&app); err != nil {
		return App{}, err
	}
	if err := app.Validate(); err != nil {
		return App{}, err
	}
	return app, nil
}

func isHTTPURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
