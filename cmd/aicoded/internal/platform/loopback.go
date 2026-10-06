package platform

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"aicoded.dev/framework/internal/errs"
)

// callbackPath is the one path of the loopback address that the platform sends the browser back
// to.
const callbackPath = "/callback"

// browserLogin signs in in a browser on this computer: it prints the address of the sign-in,
// listens for the browser's answer on a loopback address, and exchanges the code the platform
// sends the browser back with.
func (c *Client) browserLogin(ctx context.Context, org string) (*oauth2.Token, error) {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen for the browser: %w", err)
	}
	cb := &callback{state: random(), done: make(chan url.Values, 1)}
	srv := &http.Server{Handler: cb, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Close() }()

	cfg := c.config("http://" + ln.Addr().String() + callbackPath)
	verifier := oauth2.GenerateVerifier()
	authURL := cfg.AuthCodeURL(cb.state, oauth2.S256ChallengeOption(verifier), oauth2.SetAuthURLParam("org", org))
	_, _ = fmt.Fprintf(c.out, "To sign in to %s, open this address in a browser on this computer, "+
		"or run aicoded login --device on a computer without one:\n\n    %s\n", org, authURL)
	timeout := time.NewTimer(c.Timeout)
	defer timeout.Stop()
	select {
	case q := <-cb.done:
		shutdown(srv)
		return c.exchange(ctx, cfg, q, verifier)
	case <-timeout.C:
		return nil, errs.New("E-CLI-008", "the sign-in was not finished in the browser in time",
			"run aicoded login again and finish the sign-in in the browser within 5 minutes, or run aicoded login --device")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// exchange exchanges the code of the browser's answer q for tokens. An answer with an error is
// E-CLI-006.
func (c *Client) exchange(ctx context.Context, cfg *oauth2.Config, q url.Values, verifier string) (*oauth2.Token, error) {
	if q.Has("error") {
		return nil, refused(q.Get("error"), q.Get("error_description"))
	}
	code := q["code"]
	if len(code) != 1 || !visible(code[0], 4096) {
		return nil, errors.New("the platform sent the browser back with neither a code nor an error")
	}
	tok, err := cfg.Exchange(c.ctx(ctx), code[0], oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, failed("finish the sign-in", err)
	}
	return tok, nil
}

// callback takes the browser's answer to a sign-in: the first one with the sign-in's state. An
// answer with another state is refused before its code or error is read.
type callback struct {
	state string
	once  sync.Once
	done  chan url.Values
}

func (cb *callback) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != callbackPath {
		page(w, http.StatusNotFound, "There is nothing here.")
		return
	}
	if r.Method != http.MethodGet {
		page(w, http.StatusMethodNotAllowed, "There is nothing here.")
		return
	}
	q := r.URL.Query()
	if len(q["state"]) != 1 || subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(cb.state)) != 1 {
		page(w, http.StatusBadRequest, "This answer is not from the sign-in that aicoded started. Go back to the terminal.")
		return
	}
	first := false
	cb.once.Do(func() { first = true })
	switch {
	case !first:
		page(w, http.StatusBadRequest, "This sign-in is finished. You can close this window.")
		return
	case q.Has("error"):
		page(w, http.StatusOK, "The sign-in was refused. Go back to the terminal to see why; you can close this window.")
	default:
		page(w, http.StatusOK, "You can close this window and go back to the terminal.")
	}
	cb.done <- q
}

// page answers the browser with text.
func page(w http.ResponseWriter, status int, text string) {
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("Content-Security-Policy", "default-src 'none'")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, text+"\n")
}

// shutdown stops srv once it has answered the requests it is serving.
func shutdown(srv *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

// random returns 32 random bytes in base64url.
func random() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
