package store

import (
	"net/url"
	"slices"
	"strings"

	"apps-catalog/internal/catalog"
	"apps-catalog/internal/icons"
)

// LinkKeys are the links an app can have.
var LinkKeys = []string{"repo", "docs", "ci"}

// ValidateApp cleans up in and returns a message per field that needs a
// fix, or nil. st gives the known options and environments.
func ValidateApp(st Settings, in *AppInput, isNew bool) map[string]string {
	fields := map[string]string{}
	for _, s := range []*string{&in.ID, &in.Name, &in.Description, &in.Owner, &in.Contact, &in.Category,
		&in.RunsOn, &in.Version, &in.Access, &in.HealthURL, &in.Visibility, &in.IconColor, &in.Icon} {
		*s = strings.TrimSpace(*s)
	}

	if isNew && !catalog.ValidID(in.ID) {
		fields["id"] = "Use lowercase letters, numbers and single dashes"
	}
	if in.Name == "" {
		fields["name"] = "Name is required"
	}
	if isNew && in.Owner == "" {
		fields["owner"] = "Owner team is required"
	}
	if in.Category != "" && !hasOption(st.Categories, in.Category) {
		fields["category"] = "Pick a category from the list"
	}
	if in.RunsOn != "" && !hasOption(st.RunsOn, in.RunsOn) {
		fields["runs_on"] = "Pick an option from the list"
	}
	if len(in.Version) > 40 {
		fields["version"] = "Keep the version under 40 characters"
	}

	in.URLs = cleanURLs(in.URLs, "urls", fields)
	if hasOption(st.Environments, "prod") {
		if in.URLs["prod"] == "" && fields["urls.prod"] == "" {
			fields["urls.prod"] = "Production URL is required"
		}
	} else if len(in.URLs) == 0 && len(st.Environments) > 0 {
		fields["urls."+st.Environments[0].Key] = "Add at least one URL"
	}
	for k := range in.URLs {
		if !hasOption(st.Environments, k) && fields["urls."+k] == "" {
			fields["urls."+k] = "Unknown environment"
		}
	}
	in.Links = cleanURLs(in.Links, "links", fields)
	for k := range in.Links {
		if !slices.Contains(LinkKeys, k) && fields["links."+k] == "" {
			fields["links."+k] = "Unknown link"
		}
	}
	if in.HealthURL != "" && !strings.HasPrefix(in.HealthURL, "/") && !IsHTTPURL(in.HealthURL) {
		fields["health_url"] = "Use a full URL or a path like /healthz"
	}

	// groups are kept under everyone, so switching back restores them
	in.Groups = dedupe(in.Groups)
	if in.Visibility != VisibilityEveryone && in.Visibility != VisibilityGroups {
		fields["visibility"] = "Pick who can see this app"
	}
	if in.IconColor != "" && !slices.Contains(IconPalette, in.IconColor) {
		fields["icon_color"] = "Pick one of the colors"
	}
	if in.Icon != "" && !icons.Has(in.Icon) {
		fields["icon"] = "Pick an icon from the list"
	}
	if len(fields) == 0 {
		return nil
	}
	return fields
}

// cleanURLs drops empty values, adds https:// when there is no scheme and
// reports values that are not http(s) urls.
func cleanURLs(m map[string]string, prefix string, fields map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if !strings.Contains(v, "://") {
			v = "https://" + v
		}
		if !IsHTTPURL(v) {
			fields[prefix+"."+k] = "Not a valid URL"
		}
		out[k] = v
	}
	return out
}

func IsHTTPURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func hasOption(opts []Option, key string) bool {
	return slices.ContainsFunc(opts, func(o Option) bool { return o.Key == key })
}

func dedupe(items []string) []string {
	out := []string{}
	for _, s := range items {
		s = strings.TrimSpace(s)
		if s != "" && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}
