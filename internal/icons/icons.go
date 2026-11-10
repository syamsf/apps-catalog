// Package icons holds the Tabler icon set (https://tabler.io/icons) that
// admins can pick for an app. tabler.json.gz is made by generate.go.
package icons

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"io"
	"sync"
)

//go:embed tabler.json.gz
var tablerGz []byte

type tablerIcon struct {
	Name string `json:"n"`
	Body string `json:"b"`
}

var (
	once    sync.Once
	version string
	bodies  map[string]string
)

func load() {
	zr, err := gzip.NewReader(bytes.NewReader(tablerGz))
	if err != nil {
		panic("icons: " + err.Error())
	}
	var set struct {
		Version string       `json:"version"`
		Icons   []tablerIcon `json:"icons"`
	}
	if err := json.NewDecoder(zr).Decode(&set); err != nil {
		panic("icons: " + err.Error())
	}
	version = set.Version
	bodies = make(map[string]string, len(set.Icons))
	for _, ic := range set.Icons {
		bodies[ic.Name] = ic.Body
	}
}

// Version is the @tabler/icons version.
func Version() string {
	once.Do(load)
	return version
}

// Has reports if name is a Tabler icon, like "brand-docker".
func Has(name string) bool {
	once.Do(load)
	_, ok := bodies[name]
	return ok
}

// SVG returns the outline icon drawn in currentColor.
func SVG(name string) ([]byte, bool) {
	once.Do(load)
	body, ok := bodies[name]
	if !ok {
		return nil, false
	}
	return []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" ` +
		`stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + body + `</svg>`), true
}

// Index is the whole set as gzipped JSON, for the picker:
// {"version": "...", "icons": [{"n": name, "t": tags, "c": category, "b": svg body}]}
func Index() []byte { return tablerGz }

// IndexJSON is Index without the gzip, for clients that don't accept it.
func IndexJSON() ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(tablerGz))
	if err != nil {
		return nil, err
	}
	return io.ReadAll(zr)
}
