package dev

import (
	"cmp"
	"encoding/json"
	"slices"
	"strings"
)

// minSecret is the shortest secret value aicoded dev hides; hiding a shorter one would garble
// ordinary text.
const minSecret = 4

// redactor hides secret values in text.
type redactor struct {
	forms []form // longest first
}

// form is one way a secret value appears in text, and the mark that hides it.
type form struct{ text, mark string }

// newRedactor returns a redactor that replaces each secret value of names, which maps a value to
// its secret's name, with "[secret <name>]": the value and each of its lines, as they are and as
// they look inside a JSON string. Values and lines shorter than minSecret are left as they are.
func newRedactor(names map[string]string) *redactor {
	var forms []form
	for v, name := range names {
		if len(v) < minSecret {
			continue
		}
		parts := []string{v}
		if strings.Contains(v, "\n") {
			for line := range strings.Lines(v) {
				if line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"); len(line) >= minSecret {
					parts = append(parts, line)
				}
			}
		}
		for _, part := range parts {
			for _, text := range jsonForms(part) {
				forms = append(forms, form{text, "[secret " + name + "]"})
			}
		}
	}
	slices.SortFunc(forms, func(a, b form) int {
		return cmp.Or(cmp.Compare(len(b.text), len(a.text)), strings.Compare(a.text, b.text), strings.Compare(a.mark, b.mark))
	})
	return &redactor{forms: slices.Compact(forms)}
}

// String returns s with the secret values hidden. It hides every byte that any form covers:
// overlapping or adjacent matches become one mark, that of the match that starts first, the
// longest there.
func (r *redactor) String(s string) string {
	if r == nil {
		return s
	}
	type match struct {
		start, end int
		mark       string
	}
	var ms []match
	for _, f := range r.forms {
		for i := 0; ; {
			j := strings.Index(s[i:], f.text)
			if j < 0 {
				break
			}
			ms = append(ms, match{i + j, i + j + len(f.text), f.mark})
			i += j + 1
		}
	}
	if len(ms) == 0 {
		return s
	}
	slices.SortStableFunc(ms, func(a, b match) int { return cmp.Or(cmp.Compare(a.start, b.start), cmp.Compare(b.end, a.end)) })
	var b strings.Builder
	last := 0
	for k := 0; k < len(ms); {
		first, end := ms[k], ms[k].end
		for k++; k < len(ms) && ms[k].start <= end; k++ {
			end = max(end, ms[k].end)
		}
		b.WriteString(s[last:first.start])
		b.WriteString(first.mark)
		last = end
	}
	b.WriteString(s[last:])
	return b.String()
}

// value returns v, a value decoded from JSON, with the secret values hidden in every string it
// holds, keys included.
func (r *redactor) value(v any) any {
	switch v := v.(type) {
	case string:
		return r.String(v)
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, x := range v {
			out[r.String(k)] = r.value(x)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, x := range v {
			out[i] = r.value(x)
		}
		return out
	}
	return v
}

// jsonForms returns v as it is, and as it is written inside a JSON string with and without
// HTML escaping.
func jsonForms(v string) []string {
	forms := []string{v}
	for _, html := range []bool{false, true} {
		var b strings.Builder
		enc := json.NewEncoder(&b)
		enc.SetEscapeHTML(html)
		_ = enc.Encode(v)
		q := strings.TrimSuffix(b.String(), "\n")
		if q = q[1 : len(q)-1]; !slices.Contains(forms, q) {
			forms = append(forms, q)
		}
	}
	return forms
}

// cut returns the length of the longest start of s that no secret value runs past: neither one
// that s holds, nor one that the end of s may begin.
func (r *redactor) cut(s string) int {
	n := len(s)
	if r == nil {
		return n
	}
	for moved := true; moved; {
		moved = false
		for _, f := range r.forms {
			for i := max(0, n-len(f.text)+1); i < n; i++ {
				if strings.HasPrefix(f.text, s[i:min(len(s), i+len(f.text))]) {
					n, moved = i, true
					break
				}
			}
		}
	}
	return n
}
