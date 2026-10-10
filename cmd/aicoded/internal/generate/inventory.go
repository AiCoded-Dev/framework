package generate

import (
	"slices"
	"strings"

	"aicoded.dev/framework/cmd/aicoded/internal/generate/template"
)

// RouteInfo is one page or layout of an app, as its templates and the access section describe
// it. The delivery pipeline's simulated attacks are built from it.
type RouteInfo struct {
	// Path is the URL pattern, as the access section writes it, such as /trips/{id}.
	Path string `json:"path"`
	// Template is the route's template, relative to the app folder.
	Template string `json:"template"`
	// Layout tells a layout with pages below it, which only redirects to one of them and has no
	// entry in the access section.
	Layout bool `json:"layout"`
	// Params are the parameters of the URL, in path order.
	Params []ParamInfo `json:"params"`
	// Require holds the role rules a viewer must meet, as the access section writes them.
	Require []string `json:"require"`
	// Guard tells whether a Guard runs on the path, and GuardAt is the path of the route whose
	// template declares the nearest one, or "".
	Guard   bool   `json:"guard"`
	GuardAt string `json:"guard_at"`
	// Shared tells whether everyone the rules admit may see every record, as the access section
	// says.
	Shared bool `json:"shared"`
	// Live tells whether the route or a layout above it has live values or page calls, so that
	// its page opens a live connection at <path>/__ws.
	Live bool `json:"live"`
	// Calls names the page calls of the routes that render the page.
	Calls []string `json:"calls"`
	// Forms are the forms of the route's own template, in template order.
	Forms []FormInfo `json:"forms"`
}

// ParamInfo is a parameter of a URL: Kind is "number" for an n_ folder and "string" for an s_
// folder.
type ParamInfo struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// FormInfo is an <ssr:form> and its fields, in template order.
type FormInfo struct {
	Name   string      `json:"name"`
	Fields []FieldInfo `json:"fields"`
}

// FieldInfo is a form field: Kind is "input", "file", "textarea" or "select", and GoType the Go
// type its value is read into. Radio and checkbox inputs that share a name are one field.
type FieldInfo struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	GoType   string `json:"go_type"`
	Required bool   `json:"required"`
	Multiple bool   `json:"multiple"`
}

// fieldKinds names the kinds of form fields.
var fieldKinds = map[template.FormElementType]string{
	template.FormElementInput:     "input",
	template.FormElementInputFile: "file",
	template.FormElementTextarea:  "textarea",
	template.FormElementSelect:    "select",
}

// inventory returns every page and layout of routes, sorted by URL pattern.
func inventory(routes []*Route) []RouteInfo {
	byPath := routeIndex(routes)
	var out []RouteInfo
	for _, r := range routes {
		e := accessOf(r, byPath)
		info := RouteInfo{
			Path:     pattern(r.Path),
			Template: templateFile(r.Path),
			Layout:   redirects(r, byPath),
			Params:   []ParamInfo{},
			Require:  e.Require,
			Guard:    e.Guard,
			GuardAt:  guardAt(r.Path, byPath),
			Shared:   e.Shared,
			Calls:    []string{},
			Forms:    []FormInfo{},
		}
		for _, seg := range segments(r.Path) {
			if isParam(seg) {
				kind := "string"
				if strings.HasPrefix(seg, "n_") {
					kind = "number"
				}
				info.Params = append(info.Params, ParamInfo{Name: seg[2:], Kind: kind})
			}
		}
		for _, x := range chain(r.Path, byPath) {
			info.Live = info.Live || x.live != nil || len(x.Template.Calls()) > 0
		}
		info.Calls = append(info.Calls, e.Calls...)
		for _, f := range r.Template.GetForms() {
			form := FormInfo{Name: f.Name, Fields: []FieldInfo{}}
			for _, el := range f.Elements {
				form.Fields = append(form.Fields, FieldInfo{
					Name: el.Name, Kind: fieldKinds[el.Type], GoType: el.GoType, Required: el.IsRequired, Multiple: el.IsMultiple,
				})
			}
			info.Forms = append(info.Forms, form)
		}
		out = append(out, info)
	}
	slices.SortFunc(out, func(a, b RouteInfo) int { return strings.Compare(a.Path, b.Path) })
	return out
}

// guardAt returns the URL pattern of the nearest route on the path to p, p's own included, whose
// template declares guard="true", or "".
func guardAt(p string, byPath map[string]*Route) string {
	segs := segments(p)
	for i := len(segs); i >= 0; i-- {
		q := "/" + strings.Join(segs[:i], "/")
		if x := byPath[q]; x != nil && x.Template.Access() != nil && x.Template.Access().Guard {
			return pattern(q)
		}
	}
	return ""
}
