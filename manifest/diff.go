package manifest

import (
	"maps"
	"slices"
	"strings"
)

// Change is one difference between two permission lists. Key names the entry of a keyed
// field and is empty otherwise; Before is empty for what was added, After for what was removed.
type Change struct{ Field, Key, Before, After string }

// Diff returns the differences between before and after, nil when there are none.
//
// The changes come in the order of the fields, then by key. A list, such as egress, changes by
// each item removed or added, removals first, each sorted; a new order is no change, and a field
// left out equals an empty one. A keyed entry changes as a whole, written as text: a data
// source's classes, sorted; an access path's "require <rules>", then "; guard" and "; calls
// <names>" when they apply, its lists in written order; a served function's "callers <apps>",
// then "; require <rules>" and "; apps" when they apply, its lists sorted; a job's cron. The
// functions called from another app change one by one, keyed by that app.
func Diff(before, after Manifest) []Change {
	var c changes
	c.value("app", "", before.App, after.App)
	c.value("class", "", before.Class, after.Class)
	c.value("owner", "", before.Owner, after.Owner)
	c.set("audience.internal", "", before.Audience.Internal, after.Audience.Internal)
	c.set("audience.external", "", before.Audience.External, after.Audience.External)
	c.keyed("data", dataText(before.Data), dataText(after.Data))
	c.keyed("access", accessText(before.Access), accessText(after.Access))
	for _, app := range union(before.Services.Calls, after.Services.Calls) {
		c.set("services.calls", app, before.Services.Calls[app], after.Services.Calls[app])
	}
	c.keyed("services.serves", serveText(before.Services.Serves), serveText(after.Services.Serves))
	c.set("egress", "", before.Egress, after.Egress)
	be, ae := emailOf(before.Email), emailOf(after.Email)
	c.value("email.from", "", be.From, ae.From)
	c.set("email.to_domains", "", be.ToDomains, ae.ToDomains)
	c.keyed("schedule", jobText(before.Schedule), jobText(after.Schedule))
	c.set("settings", "", before.Settings, after.Settings)
	c.set("secrets", "", before.Secrets, after.Secrets)
	c.set("modules", "", before.Modules, after.Modules)
	c.value("size", "", before.Size, after.Size)
	c.value("resources", "", before.Resources, after.Resources)
	c.value("ttl", "", before.TTL, after.TTL)
	return c
}

type changes []Change

func (c *changes) value(field, key, before, after string) {
	if before != after {
		*c = append(*c, Change{field, key, before, after})
	}
}

func (c *changes) set(field, key string, before, after []string) {
	for _, v := range without(before, after) {
		c.value(field, key, v, "")
	}
	for _, v := range without(after, before) {
		c.value(field, key, "", v)
	}
}

func (c *changes) keyed(field string, before, after map[string]string) {
	for _, key := range union(before, after) {
		c.value(field, key, before[key], after[key])
	}
}

// without returns the items of a that are not in b, sorted, each once.
func without(a, b []string) []string {
	seen := make(map[string]bool, len(b))
	for _, v := range b {
		seen[v] = true
	}
	var out []string
	for _, v := range a {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	slices.Sort(out)
	return out
}

// union returns the keys of a and b, sorted, each once.
func union[V any](a, b map[string]V) []string {
	keys := slices.AppendSeq(slices.Collect(maps.Keys(a)), maps.Keys(b))
	slices.Sort(keys)
	return slices.Compact(keys)
}

func emailOf(e *Email) Email {
	if e == nil {
		return Email{}
	}
	return *e
}

func dataText(ds []Data) map[string]string {
	out := make(map[string]string, len(ds))
	for _, d := range ds {
		out[d.Source] = strings.Join(without(d.Classes, nil), ", ")
	}
	return out
}

func accessText(access map[string]Access) map[string]string {
	out := make(map[string]string, len(access))
	for path, a := range access {
		text := "require " + strings.Join(a.Require, ", ")
		if a.Guard {
			text += "; guard"
		}
		if len(a.Calls) > 0 {
			text += "; calls " + strings.Join(a.Calls, ", ")
		}
		out[path] = text
	}
	return out
}

func serveText(serves map[string]Serve) map[string]string {
	out := make(map[string]string, len(serves))
	for name, s := range serves {
		text := "callers " + strings.Join(without(s.Callers, nil), ", ")
		if len(s.Require) > 0 {
			text += "; require " + strings.Join(without(s.Require, nil), ", ")
		}
		if s.Apps {
			text += "; apps"
		}
		out[name] = text
	}
	return out
}

func jobText(jobs []Job) map[string]string {
	out := make(map[string]string, len(jobs))
	for _, j := range jobs {
		out[j.Name] = j.Cron
	}
	return out
}
