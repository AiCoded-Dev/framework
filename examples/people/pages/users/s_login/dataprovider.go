package s_login

import (
	"context"
	"errors"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	"aicoded.dev/framework/web"

	"people/deps"
)

var _ RouteDataProvider = &DP{}

// DP provides a user's card, the layout of the user's tabs.
type DP struct{ d *deps.Deps }

// NewDP returns the layout's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Data shows the user's card, with the name and size of each stored photo. A login no user has
// is 404.
func (p *DP) Data(ctx context.Context, r *web.Request, _ web.ResponseWriter, data *RouteData) error {
	u, err := p.d.User(ctx, r.URLParam("login"))
	if errors.Is(err, deps.ErrNoUser) {
		return web.NotFound()
	}
	if err != nil {
		return err
	}
	photos, err := p.d.PhotosOf(u.Login)
	if err != nil {
		return err
	}
	data.User, data.Initials, data.Photos = u, initials(u.Name), photos
	data.TabClass = func(tab string) string {
		if path.Base(r.URL.Path) == tab {
			return "active"
		}
		return ""
	}
	return nil
}

// initials returns the first letters of the first and the last word of name, in capitals.
func initials(name string) string {
	words := strings.Fields(name)
	if len(words) == 0 {
		return "?"
	}
	first, _ := utf8.DecodeRuneInString(words[0])
	if len(words) == 1 {
		return string(unicode.ToUpper(first))
	}
	last, _ := utf8.DecodeRuneInString(words[len(words)-1])
	return string(unicode.ToUpper(first)) + string(unicode.ToUpper(last))
}
