package manifest

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"slices"
	"strings"
)

// Canonical returns the form of m that Hash hashes: its JSON encoding, keyed by the names of
// aicoded.yaml, with every list sorted and each item once, and with what is empty or false left
// out. Two permission lists that differ only in the order of keys or of list items, or in
// comments, have one canonical form.
func Canonical(m Manifest) []byte {
	c := m
	c.Audience = Audience{Internal: set(m.Audience.Internal), External: set(m.Audience.External)}
	c.Data = nil
	for _, d := range m.Data {
		c.Data = append(c.Data, Data{Source: d.Source, Classes: set(d.Classes)})
	}
	slices.SortFunc(c.Data, func(a, b Data) int {
		return cmp.Or(strings.Compare(a.Source, b.Source), slices.Compare(a.Classes, b.Classes))
	})
	c.Data = slices.CompactFunc(c.Data, func(a, b Data) bool { return a.Source == b.Source && slices.Equal(a.Classes, b.Classes) })
	c.Access = mapValues(m.Access, func(a Access) Access {
		return Access{Require: set(a.Require), Guard: a.Guard, Shared: a.Shared, Calls: set(a.Calls)}
	})
	c.Services = Services{
		Calls: mapValues(m.Services.Calls, set),
		Serves: mapValues(m.Services.Serves, func(s Serve) Serve {
			return Serve{Callers: set(s.Callers), Require: set(s.Require), Apps: s.Apps}
		}),
	}
	c.Egress = set(m.Egress)
	if m.Email != nil {
		c.Email = &Email{From: m.Email.From, ToDomains: set(m.Email.ToDomains)}
	}
	c.Schedule = nil
	if len(m.Schedule) > 0 {
		c.Schedule = slices.Clone(m.Schedule)
		slices.SortFunc(c.Schedule, func(a, b Job) int {
			return cmp.Or(strings.Compare(a.Name, b.Name), strings.Compare(a.Cron, b.Cron))
		})
		c.Schedule = slices.Compact(c.Schedule)
	}
	c.Settings, c.Secrets, c.Modules = set(m.Settings), set(m.Secrets), set(m.Modules)
	b, err := json.Marshal(c)
	if err != nil {
		panic("manifest: encoding a permission list: " + err.Error())
	}
	return b
}

// Hash returns "sha256:" followed by the lowercase hex SHA-256 of Canonical(m).
func Hash(m Manifest) string {
	sum := sha256.Sum256(Canonical(m))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// set returns the items of list sorted, each once, and nil for an empty list.
func set(list []string) []string {
	if len(list) == 0 {
		return nil
	}
	return slices.Compact(slices.Sorted(slices.Values(list)))
}

// mapValues returns m with f applied to each value, and nil for an empty map.
func mapValues[V any](m map[string]V, f func(V) V) map[string]V {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]V, len(m))
	for k, v := range maps.All(m) {
		out[k] = f(v)
	}
	return out
}
