//go:build ignore

// Builds tabler.json.gz from an unpacked @tabler/icons package:
//
//	go run generate.go <package dir>
//
// Run make tabler-icons instead, it downloads the package first.
package main

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type meta struct {
	Category string `json:"category"`
	Tags     []any  `json:"tags"`
}

type icon struct {
	Name     string `json:"n"`
	Tags     string `json:"t,omitempty"`
	Category string `json:"c,omitempty"`
	Body     string `json:"b"`
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run generate.go <@tabler/icons package dir>")
		os.Exit(2)
	}
	dir := os.Args[1]
	var pkg struct{ Version string }
	var metas map[string]meta
	var nodes map[string][][2]any
	for file, dst := range map[string]any{"package.json": &pkg, "icons.json": &metas, "tabler-nodes-outline.json": &nodes} {
		data, err := os.ReadFile(filepath.Join(dir, file))
		must(err)
		must(json.Unmarshal(data, dst))
	}

	out := struct {
		Version string `json:"version"`
		Icons   []icon `json:"icons"`
	}{Version: pkg.Version}
	for name, els := range nodes {
		var b strings.Builder
		for _, el := range els {
			tag, _ := el[0].(string)
			attrs, _ := el[1].(map[string]any)
			keys := make([]string, 0, len(attrs))
			for k := range attrs {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			b.WriteString("<" + tag)
			for _, k := range keys {
				fmt.Fprintf(&b, ` %s="%s"`, k, html.EscapeString(fmt.Sprint(attrs[k])))
			}
			b.WriteString("/>")
		}
		m := metas[name]
		tags := make([]string, 0, len(m.Tags))
		for _, t := range m.Tags {
			tags = append(tags, fmt.Sprint(t))
		}
		out.Icons = append(out.Icons, icon{Name: name, Tags: strings.Join(tags, " "), Category: m.Category, Body: b.String()})
	}
	sort.Slice(out.Icons, func(i, j int) bool { return out.Icons[i].Name < out.Icons[j].Name })

	f, err := os.Create("tabler.json.gz")
	must(err)
	zw, err := gzip.NewWriterLevel(f, gzip.BestCompression)
	must(err)
	enc := json.NewEncoder(zw)
	enc.SetEscapeHTML(false)
	must(enc.Encode(out))
	must(zw.Close())
	must(f.Close())
	fmt.Printf("tabler.json.gz: %d icons from @tabler/icons %s\n", len(out.Icons), pkg.Version)
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
