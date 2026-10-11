package server

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/app"
	"aicoded.dev/framework/runnerproto"
	"aicoded.dev/framework/runnerproto/socket"
	"aicoded.dev/framework/runnerproto/viewer"
	"aicoded.dev/framework/web"
)

// formPage is a page with the form "f" that shows the form token of its viewer.
type formPage struct{}

func (formPage) Access() web.Access       { return web.Access{Roles: []string{"*"}} }
func (formPage) Layout() bool             { return false }
func (formPage) NewState() web.RouteState { return &formState{} }

type formState struct {
	web.Frame
	token string
}

func (*formState) Guard(context.Context, *web.Request) error { return nil }

func (s *formState) InitForms(_ context.Context, r *web.Request, _ web.ResponseWriter) error {
	s.token = r.CSRFToken()
	return nil
}

func (*formState) SubmitForm(_ context.Context, _ *web.Request, _ web.ResponseWriter, id string) (bool, error) {
	return id == "f", nil
}

func (*formState) Data(context.Context, *web.Request, web.ResponseWriter) error { return nil }

func (*formState) DefaultRoute(context.Context, *web.Request) (string, error) { return "", nil }

func (s *formState) Write(w io.Writer) error {
	_, err := io.WriteString(w, s.token)
	return err
}

// runFormApp runs an app of notes with formPage under a server for it with csrfKey and
// viewerKey, and returns a client of its app.sock.
func runFormApp(t *testing.T, key []byte, viewerKey ed25519.PublicKey) *http.Client {
	opts := options(t, newFakes())
	opts.CSRFKey, opts.ViewerKeys = key, []ed25519.PublicKey{viewerKey}
	s := start(t, opts)
	t.Setenv(runnerproto.EnvRunnerDir, opts.Dir)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- app.Run(ctx, app.Options{Handler: web.New(map[string]web.Route{"/": formPage{}}, web.Options{})})
	}()
	t.Cleanup(func() { cancel(); <-done })
	select {
	case <-s.Ready():
	case err := <-done:
		t.Fatalf("the app stopped before it was ready: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("the app did not report ready")
	}
	c := socket.Client(filepath.Join(opts.Dir, runnerproto.AppSocket))
	t.Cleanup(c.CloseIdleConnections)
	return c
}

// visit sends a request for / as alice, with form as the body of a POST, and returns the status
// and the body of the answer.
func visit(t *testing.T, c *http.Client, signer ed25519.PrivateKey, form url.Values) (int, string) {
	now := time.Now()
	token, err := viewer.Sign(signer, viewer.Claims{Subject: "alice", Audience: "notes", IssuedAt: now.Unix(), Expires: now.Add(time.Minute).Unix()})
	require.NoError(t, err)
	method, body := http.MethodGet, io.Reader(http.NoBody)
	if form != nil {
		method, body = http.MethodPost, strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(t.Context(), method, "http://notes/", body)
	require.NoError(t, err)
	req.Header.Set(viewer.Header, token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(out)
}

func TestServersWithOneCSRFKeyShareFormTokens(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	first := runFormApp(t, csrfKey, pub)
	code, token := visit(t, first, priv, nil)
	require.Equal(t, http.StatusOK, code)
	require.NotEmpty(t, token)
	post := url.Values{web.CSRFField: {token}, web.FormField: {"f"}}

	second := runFormApp(t, bytes.Clone(csrfKey), pub)
	code, _ = visit(t, second, priv, post)
	assert.Equal(t, http.StatusOK, code, "an instance with the same key takes the form")

	other := runFormApp(t, bytes.Repeat([]byte{8}, MinCSRFKey), pub)
	code, _ = visit(t, other, priv, post)
	assert.Equal(t, http.StatusForbidden, code, "an instance with another key refuses it")
}
