package catalog

import (
	"errors"
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"
)

// FileVersion is the import file format version.
const FileVersion = 1

// Entry is one app in an import file. Values are as written, the store
// matches them to options and checks them.
type Entry struct {
	Line        int
	ID          string
	Name        string
	Description string
	Owner       string
	Contact     string
	Category    string
	RunsOn      string
	Version     string
	StartHere   bool
	Icon        string
	Access      string
	HealthURL   string
	IconColor   string
	URLs        map[string]string
	Links       map[string]string

	// VisibleTo is nil when the field is left out, ["everyone"] for
	// everyone, otherwise group names. An empty list means admins only.
	VisibleTo []string

	// line of each field, "urls.prod" for nested keys
	Lines map[string]int
}

// Problem is something wrong in an import file.
type Problem struct {
	Line    int    `json:"line"`
	App     string `json:"app"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

var entryFields = map[string]bool{
	"id": true, "name": true, "description": true, "owner": true, "contact": true, "category": true,
	"runs_on": true, "access": true, "health_url": true, "icon_color": true, "urls": true, "links": true,
	"visible_to": true, "version": true, "start_here": true, "icon": true,
}

// ParseFile reads an import file:
//
//	version: 1
//	apps:
//	  - id: grafana
//	    name: Grafana
//	    ...
//
// It returns every problem it finds, not only the first one. Entries with
// problems are still returned so a preview can list them.
func ParseFile(data []byte) ([]Entry, []Problem) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, []Problem{yamlProblem(err)}
	}
	if len(root.Content) == 0 {
		return nil, []Problem{{Line: 1, Message: "The file is empty"}}
	}
	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return nil, []Problem{{Line: doc.Line, Message: "Expected version and apps at the top of the file"}}
	}

	var (
		probs   []Problem
		apps    *yaml.Node
		version *yaml.Node
	)
	for i := 0; i+1 < len(doc.Content); i += 2 {
		k, v := doc.Content[i], doc.Content[i+1]
		switch k.Value {
		case "version":
			version = v
		case "apps":
			apps = v
		default:
			probs = append(probs, Problem{Line: k.Line, Field: k.Value, Message: fmt.Sprintf("Unknown field %q", k.Value)})
		}
	}
	switch {
	case version == nil:
		probs = append(probs, Problem{Line: doc.Line, Field: "version", Message: fmt.Sprintf("Add version: %d at the top", FileVersion)})
	case version.Value != fmt.Sprint(FileVersion):
		probs = append(probs, Problem{Line: version.Line, Field: "version",
			Message: fmt.Sprintf("Version %q is not supported, use %d", version.Value, FileVersion)})
	}
	if apps == nil || (apps.Kind == yaml.ScalarNode && apps.Tag == "!!null") {
		return nil, append(probs, Problem{Line: doc.Line, Field: "apps", Message: "Add an apps list"})
	}
	if apps.Kind != yaml.SequenceNode {
		return nil, append(probs, Problem{Line: apps.Line, Field: "apps", Message: "apps must be a list"})
	}
	if len(apps.Content) == 0 {
		probs = append(probs, Problem{Line: apps.Line, Field: "apps", Message: "The apps list is empty"})
	}

	entries := make([]Entry, 0, len(apps.Content))
	seen := map[string]int{}
	for _, n := range apps.Content {
		e, p := parseEntry(n)
		probs = append(probs, p...)
		if e.ID != "" {
			if prev, dup := seen[e.ID]; dup {
				probs = append(probs, Problem{Line: e.Line, App: e.ID, Field: "id",
					Message: fmt.Sprintf("ID %q is also used on line %d", e.ID, prev)})
			} else {
				seen[e.ID] = e.Line
			}
		}
		entries = append(entries, e)
	}
	return entries, probs
}

func parseEntry(n *yaml.Node) (Entry, []Problem) {
	e := Entry{Line: n.Line, Lines: map[string]int{}}
	if n.Kind != yaml.MappingNode {
		return e, []Problem{{Line: n.Line, Message: "Each app must be a set of fields like id: and name:"}}
	}
	var probs []Problem
	// the id is read first so every problem can name the app
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == "id" && n.Content[i+1].Kind == yaml.ScalarNode {
			e.ID = strings.TrimSpace(n.Content[i+1].Value)
		}
	}
	bad := func(line int, field, msg string) {
		probs = append(probs, Problem{Line: line, App: e.ID, Field: field, Message: msg})
	}
	scalar := func(k, v *yaml.Node, dst *string) {
		if v.Kind != yaml.ScalarNode || v.Tag == "!!null" {
			if v.Tag != "!!null" {
				bad(k.Line, k.Value, k.Value+" must be text")
			}
			return
		}
		*dst = v.Value
	}
	urlMap := func(k, v *yaml.Node) map[string]string {
		if v.Tag == "!!null" {
			return nil
		}
		if v.Kind != yaml.MappingNode {
			bad(k.Line, k.Value, k.Value+" must be a list of name: url pairs")
			return nil
		}
		out := map[string]string{}
		for j := 0; j+1 < len(v.Content); j += 2 {
			mk, mv := v.Content[j], v.Content[j+1]
			e.Lines[k.Value+"."+mk.Value] = mk.Line
			if mv.Kind != yaml.ScalarNode {
				bad(mk.Line, k.Value+"."+mk.Value, "Must be a URL")
				continue
			}
			out[mk.Value] = mv.Value
		}
		return out
	}

	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if !entryFields[k.Value] {
			bad(k.Line, k.Value, fmt.Sprintf("Unknown field %q", k.Value))
			continue
		}
		e.Lines[k.Value] = k.Line
		switch k.Value {
		case "id":
			scalar(k, v, &e.ID)
		case "name":
			scalar(k, v, &e.Name)
		case "description":
			scalar(k, v, &e.Description)
		case "owner":
			scalar(k, v, &e.Owner)
		case "contact":
			scalar(k, v, &e.Contact)
		case "category":
			scalar(k, v, &e.Category)
		case "runs_on":
			scalar(k, v, &e.RunsOn)
		case "version":
			scalar(k, v, &e.Version)
		case "icon":
			scalar(k, v, &e.Icon)
		case "start_here":
			if v.Kind != yaml.ScalarNode || v.Decode(&e.StartHere) != nil {
				bad(k.Line, k.Value, "start_here must be true or false")
			}
		case "access":
			scalar(k, v, &e.Access)
		case "health_url":
			scalar(k, v, &e.HealthURL)
		case "icon_color":
			scalar(k, v, &e.IconColor)
		case "urls":
			e.URLs = urlMap(k, v)
		case "links":
			e.Links = urlMap(k, v)
		case "visible_to":
			switch {
			case v.Kind == yaml.ScalarNode && v.Tag != "!!null":
				e.VisibleTo = []string{v.Value}
			case v.Kind == yaml.SequenceNode:
				e.VisibleTo = []string{}
				for _, g := range v.Content {
					if g.Kind != yaml.ScalarNode {
						bad(g.Line, "visible_to", "Each group must be a name")
						continue
					}
					e.VisibleTo = append(e.VisibleTo, g.Value)
				}
			default:
				bad(k.Line, "visible_to", "Use everyone or a list of groups")
			}
		}
	}
	return e, probs
}

func yamlProblem(err error) Problem {
	msg := err.Error()
	line := 0
	var te *yaml.TypeError
	if errors.As(err, &te) && len(te.Errors) > 0 {
		msg = te.Errors[0]
	}
	// "yaml: line 4: did not find expected key"
	if rest, ok := strings.CutPrefix(msg, "yaml: line "); ok {
		if i := strings.Index(rest, ":"); i > 0 {
			fmt.Sscan(rest[:i], &line)
			msg = strings.TrimSpace(rest[i+1:])
		}
	}
	msg = strings.TrimPrefix(msg, "yaml: ")
	return Problem{Line: line, Message: "Not valid YAML: " + msg}
}
