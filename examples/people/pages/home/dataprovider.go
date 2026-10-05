package home

import (
	"context"
	"net/http"
	"unicode/utf8"

	"aicoded.dev/framework/auth"
	"aicoded.dev/framework/web"

	"people/deps"
)

var _ RouteDataProvider = &DP{}

// DP provides the home page.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Data fills the showcase, and counts the open home pages with this one.
func (p *DP) Data(ctx context.Context, _ *web.Request, _ web.ResponseWriter, data *RouteData) error {
	data.Intro = web.JoinHTML(web.HTMLConst("You are signed in as <strong>"), web.EscapeHTML(auth.Viewer(ctx).Name), web.HTMLConst("</strong>."))
	data.VisitorsOnline = p.d.HomePages.TotalSubs() + 1
	data.VisitorsTone = tone(data.VisitorsOnline)
	data.Price, data.Quantity = 9.99, 3
	data.UserName, data.UserAge = "Alice", 25
	data.Status = "active"
	data.Fruits = []string{"Apple", "Banana", "Cherry"}
	data.Langs = []string{"Go", "TypeScript", "Rust"}
	return nil
}

// Subscribe counts this page among the open home pages while it is open, and shows the count
// whenever a home page opens or closes.
func (p *DP) Subscribe(ctx context.Context, _ *web.Request, state *ReactiveState) error {
	sub := p.d.HomePages.Subscribe()
	defer func() {
		sub.Close()
		p.d.HomePages.Publish(struct{}{})
	}()
	p.d.HomePages.Publish(struct{}{})
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-sub.Updates():
			n := p.d.HomePages.TotalSubs()
			state.SetVisitorsOnline(n)
			state.SetVisitorsTone(tone(n))
		}
	}
}

// ValidateDisplayName accepts a display name of at most 50 characters.
func (p *DP) ValidateDisplayName(_ context.Context, _ *web.Request, val string) (string, error) {
	if utf8.RuneCountInString(val) > 50 {
		return "", web.Error(http.StatusUnprocessableEntity, "Use at most 50 characters.")
	}
	return val, nil
}

// tone returns the class of the badge that shows n open home pages.
func tone(n int) string {
	switch {
	case n <= 1:
		return "alone"
	case n < 5:
		return "few"
	default:
		return "many"
	}
}
