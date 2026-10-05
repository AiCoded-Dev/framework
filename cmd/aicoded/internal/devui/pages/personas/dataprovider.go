package personas

import (
	"context"
	"strings"

	"aicoded.dev/framework/web"

	"aicoded.dev/framework/cmd/aicoded/internal/devui/deps"
)

var _ RouteDataProvider = &DP{}

// DP provides the personas and the apps whose persona can be chosen.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Data lists the personas, and the persona page of every app with pages.
func (p *DP) Data(ctx context.Context, _ *web.Request, _ web.ResponseWriter, data *RouteData) error {
	st, err := p.d.Backend.Status(ctx)
	if err != nil {
		return err
	}
	for _, ps := range p.d.Backend.Personas() {
		data.Personas = append(data.Personas, Persona{Name: ps.Name, Roles: strings.Join(ps.Roles, ", "), Groups: strings.Join(ps.Groups, ", ")})
	}
	for _, a := range st.Apps {
		if a.URL != "" {
			data.Switchers = append(data.Switchers, Switcher{App: a.Name, URL: strings.TrimSuffix(a.URL, "/") + "/_aicoded/persona"})
		}
	}
	return nil
}
