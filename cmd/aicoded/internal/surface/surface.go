// Package surface computes an app's surface, its size: the larger of the number of its entry
// points and the number of its effects.
package surface

import (
	"fmt"

	"aicoded.dev/framework/lint"
	"aicoded.dev/framework/manifest"
)

// Surface is the size of an app.
type Surface struct {
	// Size is the larger of EntryPoints and Effects.
	Size int `json:"size"`
	// EntryPoints counts the pages, the page calls, once for each page that offers one, the
	// functions the app serves other apps and its scheduled jobs, from the access, services and
	// schedule sections of its permission list.
	EntryPoints int `json:"entry_points"`
	// Effects counts the tables its SQL writes, once for each kind of write, the connectors it
	// declares, the outside services it may call and the domains it may send mail to.
	Effects int `json:"effects"`
	// Writes lists the writes of its SQL as "INSERT notes", sorted.
	Writes []string `json:"writes,omitempty"`
}

// Of returns the surface of the app whose permission list is m and whose SQL makes writes.
func Of(m manifest.Manifest, writes []lint.Write) Surface {
	var s Surface
	for _, a := range m.Access {
		s.EntryPoints += 1 + len(a.Calls)
	}
	s.EntryPoints += len(m.Services.Serves) + len(m.Schedule)
	for _, w := range writes {
		s.Writes = append(s.Writes, w.Op+" "+w.Table)
	}
	s.Effects = len(writes) + len(m.Connectors()) + len(m.Egress)
	if m.Email != nil {
		s.Effects += len(m.Email.ToDomains)
	}
	s.Size = max(s.EntryPoints, s.Effects)
	return s
}

// String returns the surface as aicoded describe and check print it.
func (s Surface) String() string {
	return fmt.Sprintf("surface %d (%s, %s)", s.Size, count(s.EntryPoints, "entry point"), count(s.Effects, "effect"))
}

// count returns n and the noun, in the plural unless n is 1.
func count(n int, noun string) string {
	if n != 1 {
		noun += "s"
	}
	return fmt.Sprintf("%d %s", n, noun)
}
