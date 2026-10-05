package n_id

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/web"
)

// spy records whether the rename form was processed.
type spy struct {
	DP
	processed bool
}

func (p *spy) ProcessRename(context.Context, *web.Request, web.ResponseWriter, *FormRenameValues) error {
	p.processed = true
	return nil
}

// submit posts the rename form to a new state of the page.
func submit(t *testing.T, contentType string, values url.Values) (*spy, error) {
	dp := &spy{}
	s := NewRoute(dp).NewState().(*state)
	hr := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/notes/1", strings.NewReader(values.Encode()))
	hr.Header.Set("Content-Type", contentType)
	require.NoError(t, hr.ParseForm())
	ok, err := s.SubmitForm(t.Context(), &web.Request{Request: hr}, nil, routeKey+".rename")
	assert.True(t, ok, "the page owns the form")
	return dp, err
}

func TestValidFormIsProcessed(t *testing.T) {
	dp, err := submit(t, "application/x-www-form-urlencoded", url.Values{"title": {"Second"}})
	require.NoError(t, err)
	assert.True(t, dp.processed)
}

func TestInvalidFormIsNotProcessed(t *testing.T) {
	dp, err := submit(t, "application/x-www-form-urlencoded", url.Values{"title": {""}})
	require.NoError(t, err)
	assert.False(t, dp.processed, "the required title is empty")
}

func TestFormInTheWrongFormatIsRefused(t *testing.T) {
	dp, err := submit(t, "multipart/form-data; boundary=x", nil)
	var he *web.HTTPError
	require.ErrorAs(t, err, &he)
	assert.Equal(t, http.StatusBadRequest, he.Status)
	assert.False(t, dp.processed)
}

// A slug that starts with "/", or is empty, never turns the short link into a link to another
// site.
func TestShortLinkStaysOnSite(t *testing.T) {
	for slug, want := range map[string]string{
		"/evil.example": `<a href="/%2fevil.example">`,
		"":              `<a href="/.">`,
		"n1":            `<a href="/n1">`,
	} {
		s := NewRoute(&DP{}).NewState().(*state)
		s.RouteData.Note = Note{Slug: slug}
		var b strings.Builder
		require.NoError(t, s.Write(&b))
		assert.Contains(t, b.String(), want, slug)
	}
}
