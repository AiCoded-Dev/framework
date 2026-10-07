package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"aicoded.dev/framework/internal/errs"
)

const (
	clientID = "aicoded"
	// scope is the scope aicoded asks for; the platform decides the scopes it grants.
	scope = "app:create"
	// refreshBefore is how long before it expires an access token is refreshed.
	refreshBefore = 30 * time.Second
	// maxAnswer is the largest answer read from the platform.
	maxAnswer = 1 << 20
	// maxCodeWait is the longest a sign-in with a code waits, whatever the platform says.
	maxCodeWait = 15 * time.Minute
)

// ErrNoOrg is Login's error when it has no organisation to sign in to: the first login needs one.
var ErrNoOrg = errors.New("no organisation to sign in to")

// ErrCancelled is Login's error when its context ends before the sign-in does.
var ErrCancelled = errors.New("sign-in cancelled")

// Whoami is who a sign-in is for.
type Whoami struct {
	Org    string   `json:"org"`
	Email  string   `json:"email"`
	Scopes []string `json:"scopes"`
}

// Client signs a builder in to the platform at one address and keeps the sign-in.
type Client struct {
	// HTTP makes the requests to the platform. It must not follow redirects.
	HTTP *http.Client
	// Timeout is how long a sign-in in the browser may take.
	Timeout time.Duration

	address string
	out     io.Writer
}

// New returns a client of the platform at address, which Address returned, that gives a sign-in
// in the browser 5 minutes. It tells the person what to do to sign in on out.
func New(address string, out io.Writer) *Client {
	return &Client{
		HTTP: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}},
		Timeout: 5 * time.Minute,
		address: address,
		out:     out,
	}
}

// Login signs in to the organisation org: in a browser on this computer, or, with device, with a
// code entered in a browser on any device. An empty org is the organisation of the
// last sign-in; without one, Login returns ErrNoOrg. It keeps the new sign-in, revokes the one it
// replaces, and returns who it is for. When ctx ends first, it returns ErrCancelled.
func (c *Client) Login(ctx context.Context, org string, device bool) (Whoami, error) {
	old, err := Load(c.address)
	if err != nil {
		return Whoami{}, err
	}
	if org == "" {
		org = old.Org
	}
	if org == "" {
		return Whoami{}, ErrNoOrg
	}
	if !ValidOrg(org) {
		return Whoami{}, fmt.Errorf("%q is not an organisation's name: 2 to 39 lowercase letters, digits and dashes", clean(org, 64))
	}
	var tok *oauth2.Token
	if device {
		tok, err = c.codeLogin(ctx, org)
	} else {
		tok, err = c.browserLogin(ctx, org)
	}
	if err != nil && ctx.Err() != nil {
		return Whoami{}, ErrCancelled
	}
	if err != nil {
		return Whoami{}, err
	}
	cr, err := credentials(org, tok)
	if err != nil {
		return Whoami{}, err
	}
	replaced, err := swap(c.address, cr)
	if err != nil {
		return Whoami{}, err
	}
	if replaced.RefreshToken != "" {
		_ = c.revoke(ctx, replaced.RefreshToken)
	}
	return c.whoami(ctx, cr.AccessToken)
}

// userCode is a code the person enters in the browser.
var userCode = regexp.MustCompile(`^[A-Z]{4}-[A-Z]{4}$`)

// codeLogin signs in with the device flow: it shows the page to open and the code to enter
// there, and polls as the platform says until the sign-in ends.
func (c *Client) codeLogin(ctx context.Context, org string) (*oauth2.Token, error) {
	cfg := c.config("")
	da, err := cfg.DeviceAuth(c.ctx(ctx), oauth2.SetAuthURLParam("org", org))
	if err != nil {
		return nil, failed("start the sign-in", err)
	}
	if !visible(da.DeviceCode, 4096) || !userCode.MatchString(da.UserCode) || !visible(da.VerificationURI, 512) ||
		!strings.HasPrefix(da.VerificationURI, c.address+"/") || da.Interval < 0 || da.Interval > 60 {
		return nil, errors.New("the platform's answer to the sign-in with a code is not valid")
	}
	_, _ = fmt.Fprintf(c.out, "To sign in to %s, open\n\n    %s\n\nin a browser on any device, and enter the code %s\n",
		org, da.VerificationURI, da.UserCode)
	wait, cancel := context.WithTimeout(c.ctx(ctx), maxCodeWait)
	defer cancel()
	tok, err := cfg.DeviceAccessToken(wait, da)
	var re *oauth2.RetrieveError
	switch {
	case err == nil:
		return tok, nil
	case ctx.Err() != nil:
		return nil, ctx.Err()
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &re) && re.ErrorCode == "expired_token":
		return nil, errs.New("E-CLI-008", "the code expired before the sign-in was finished",
			"run aicoded login again, and finish the sign-in before the code expires")
	}
	return nil, failed("finish the sign-in", err)
}

// credentials returns the sign-in to org that tok holds.
func credentials(org string, tok *oauth2.Token) (Credentials, error) {
	if !visible(tok.AccessToken, 4096) || !visible(tok.RefreshToken, 4096) || !strings.EqualFold(tok.Type(), "Bearer") ||
		tok.Expiry.IsZero() {
		return Credentials{}, errors.New("the platform's tokens are not valid")
	}
	return Credentials{Org: org, AccessToken: tok.AccessToken, RefreshToken: tok.RefreshToken, Expires: tok.Expiry.UTC().Round(0)}, nil
}

// Token returns the access token of the sign-in to the platform, refreshed first when it expires
// within 30 seconds. It holds the lock on the credentials file meanwhile, so that of the aicoded
// commands that run at once only one refreshes the sign-in, and the others use its new token.
// Without a sign-in it is E-CLI-004. When the platform no longer refreshes the sign-in, Token
// deletes its tokens, keeps its organisation, and returns E-CLI-004 too. When the platform cannot
// refresh it right now, Token keeps it and returns E-CLI-009.
func (c *Client) Token(ctx context.Context) (string, error) {
	unlock, err := lock(ctx)
	if err != nil {
		return "", err
	}
	defer unlock()
	cr, err := Load(c.address)
	if err != nil {
		return "", err
	}
	if cr.AccessToken == "" || cr.RefreshToken == "" {
		return "", signedOut("not signed in to the platform at " + c.address)
	}
	if time.Until(cr.Expires) > refreshBefore {
		return cr.AccessToken, nil
	}
	tok, err := c.config("").TokenSource(c.ctx(ctx), &oauth2.Token{RefreshToken: cr.RefreshToken}).Token()
	var re *oauth2.RetrieveError
	switch {
	case errors.As(err, &re) && re.ErrorCode == "invalid_grant":
		if err := save(c.address, Credentials{Org: cr.Org}); err != nil {
			return "", err
		}
		msg := "your sign-in to the platform at " + c.address + " has ended"
		if re.ErrorDescription != "" {
			msg += ": " + clean(re.ErrorDescription, 300)
		}
		return "", signedOut(msg)
	case err != nil && ctx.Err() != nil:
		return "", ctx.Err()
	case err != nil && temporary(err):
		return "", notRefreshed(err)
	case err != nil:
		return "", failed("refresh the sign-in", err)
	}
	next, err := credentials(cr.Org, tok)
	if err != nil {
		return "", err
	}
	if err := save(c.address, next); err != nil {
		return "", err
	}
	return next.AccessToken, nil
}

// temporary reports whether err, of a request to the platform's OAuth endpoints, is a failure
// that may pass: no connection, no answer in time, an answer with a 5xx status, or the error
// server_error or temporarily_unavailable.
func temporary(err error) bool {
	var re *oauth2.RetrieveError
	if errors.As(err, &re) {
		return re.ErrorCode == "server_error" || re.ErrorCode == "temporarily_unavailable" ||
			re.Response != nil && re.Response.StatusCode >= 500
	}
	var ue *url.Error
	var oe *net.OpError
	return errors.As(err, &ue) &&
		(ue.Timeout() || errors.As(ue.Err, &oe) || errors.Is(ue.Err, io.EOF) || errors.Is(ue.Err, io.ErrUnexpectedEOF))
}

// notRefreshed is E-CLI-009 for err, a temporary failure of a refresh.
func notRefreshed(err error) error {
	msg := "the platform could not refresh your sign-in right now"
	var re *oauth2.RetrieveError
	var ue *url.Error
	switch {
	case errors.As(err, &re) && re.Response != nil:
		msg += fmt.Sprintf(": it answered %d", re.Response.StatusCode)
		if re.ErrorCode != "" {
			msg += ": " + clean(re.ErrorCode, 64)
		}
		if re.ErrorDescription != "" {
			msg += ": " + clean(re.ErrorDescription, 300)
		}
	case errors.As(err, &ue):
		msg += ": " + clean(ue.Err.Error(), 300)
	}
	return errs.New("E-CLI-009", msg, "try again in a minute: your sign-in is kept")
}

func signedOut(msg string) error {
	return errs.New("E-CLI-004", msg, "run aicoded login --org <your organisation>")
}

// Whoami asks the platform who the sign-in is for, after refreshing it when needed.
func (c *Client) Whoami(ctx context.Context) (Whoami, error) {
	tok, err := c.Token(ctx)
	if err != nil {
		return Whoami{}, err
	}
	return c.whoami(ctx, tok)
}

func (c *Client) whoami(ctx context.Context, token string) (Whoami, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.address+"/v1/whoami", nil)
	if err != nil {
		return Whoami{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	body, status, err := c.do(req)
	if err != nil {
		return Whoami{}, err
	}
	switch status {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return Whoami{}, signedOut("the platform at " + c.address + " no longer accepts your sign-in")
	default:
		return Whoami{}, fmt.Errorf("the platform answered %d%s", status, reason(body))
	}
	var w Whoami
	if err := json.Unmarshal(body, &w); err != nil || w.Org == "" || w.Email == "" {
		return Whoami{}, errors.New("the platform's answer to whoami is not valid")
	}
	w.Org, w.Email = clean(w.Org, 64), clean(w.Email, 320)
	scopes := make([]string, 0, len(w.Scopes))
	for _, s := range w.Scopes {
		scopes = append(scopes, clean(s, 128))
	}
	w.Scopes = scopes
	return w, nil
}

// Logout deletes the tokens of the sign-in, keeping its organisation for the next login, and
// revokes the sign-in at the platform (RFC 7009). It deletes them even when the platform cannot
// revoke them, and then returns revoked false. signedIn is false when there was no sign-in.
func (c *Client) Logout(ctx context.Context) (signedIn, revoked bool, err error) {
	unlock, err := lock(ctx)
	if err != nil {
		return false, false, err
	}
	cr, err := Load(c.address)
	if err == nil && cr.Org == "" {
		err = remove(c.address)
	} else if err == nil {
		err = save(c.address, Credentials{Org: cr.Org})
	}
	unlock()
	if cr.RefreshToken != "" {
		signedIn = true
		revoked = c.revoke(ctx, cr.RefreshToken) == nil
	}
	return signedIn, revoked, err
}

// revoke asks the platform to revoke the refresh token and the sign-in it belongs to.
func (c *Client) revoke(ctx context.Context, refresh string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	form := url.Values{"token": {refresh}, "token_type_hint": {"refresh_token"}, "client_id": {clientID}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.address+"/oauth/revoke", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_, status, err := c.do(req)
	if err == nil && status != http.StatusOK {
		err = fmt.Errorf("the platform answered %d", status)
	}
	return err
}

// do sends req and returns the answer's body and status. It follows no redirect.
func (c *Client) do(req *http.Request) ([]byte, int, error) {
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("reach the platform: %s", clean(err.Error(), 300))
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAnswer))
	if err != nil {
		return nil, 0, fmt.Errorf("read the platform's answer: %s", clean(err.Error(), 300))
	}
	return body, resp.StatusCode, nil
}

// config returns the OAuth configuration of aicoded at the platform, with the loopback address
// redirect.
func (c *Client) config(redirect string) *oauth2.Config {
	return &oauth2.Config{ClientID: clientID, RedirectURL: redirect, Scopes: []string{scope},
		Endpoint: oauth2.Endpoint{AuthURL: c.address + "/oauth/authorize", TokenURL: c.address + "/oauth/token",
			DeviceAuthURL: c.address + "/oauth/device", AuthStyle: oauth2.AuthStyleInParams}}
}

// ctx returns ctx with the client's HTTP client, which oauth2 sends its requests with.
func (c *Client) ctx(ctx context.Context) context.Context {
	return context.WithValue(ctx, oauth2.HTTPClient, c.HTTP)
}

// failed returns the error of a request to the platform's OAuth endpoints: E-CLI-006 when the
// platform refused it with an RFC 6749 error.
func failed(what string, err error) error {
	var re *oauth2.RetrieveError
	switch {
	case errors.As(err, &re) && re.ErrorCode != "":
		return refused(re.ErrorCode, re.ErrorDescription)
	case errors.As(err, &re) && re.Response != nil:
		return fmt.Errorf("%s: the platform answered %d", what, re.Response.StatusCode)
	}
	return fmt.Errorf("%s: %s", what, clean(err.Error(), 300))
}

// refused is E-CLI-006, quoting the platform's error and its description.
func refused(code, description string) error {
	fix := "run aicoded login again; if the platform refuses you again, ask your administrator"
	switch {
	case code == "access_denied" &&
		(strings.HasPrefix(description, "not in the builder group") || strings.HasPrefix(description, "not invited to")):
		fix = "ask your administrator to add you to the builder group"
	case code == "invalid_request" && description == "no organisation has that name":
		fix = "check the organisation's name with your administrator, then run aicoded login --org <organisation>"
	}
	msg := "the platform refused the sign-in: " + clean(code, 64)
	if description != "" {
		msg += ": " + clean(description, 300)
	}
	return errs.New("E-CLI-006", msg, fix)
}

// reason returns ": <error>: <description>" of an RFC 6749 error answer, or "".
func reason(body []byte) string {
	var e struct {
		Code        string `json:"error"`
		Description string `json:"error_description"`
	}
	if json.Unmarshal(body, &e) != nil || e.Code == "" {
		return ""
	}
	return ": " + clean(e.Code, 64) + ": " + clean(e.Description, 300)
}
