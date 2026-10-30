package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"aicoded.dev/framework/manifest"
)

// hostName matches the host of a preview: lowercase DNS labels joined by dots.
var hostName = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$`)

// Preview is the preview of an app on the platform: the publish it runs, at Host, which URL
// opens, from Started until Expires. Reset is whether this start deleted the preview's data,
// since the publish it ran had schema changes that this one does not continue.
type Preview struct {
	App     string    `json:"app"`
	Host    string    `json:"host"`
	URL     string    `json:"url"`
	Publish string    `json:"publish"`
	Started time.Time `json:"started"`
	Expires time.Time `json:"expires"`
	Reset   bool      `json:"reset"`
}

// PreviewStart runs the publish id as its app's preview on the platform, in place of the version
// the preview runs, and returns the preview. A publish that has not passed its checks is
// E-REL-029, and one whose permission list declares values not set for the preview E-REL-030;
// an id the platform does not know is E-PUB-013, and a publish of an app the builder does not
// own E-PUB-005, as the platform's answer says.
func (c *Client) PreviewStart(ctx context.Context, id string) (Preview, error) {
	body, err := json.Marshal(map[string]string{"publish": id})
	if err != nil {
		return Preview{}, err
	}
	status, answer, err := c.call(ctx, c.HTTP, http.MethodPost, "/v1/previews/start", "application/json", body, maxAnswer)
	if err != nil {
		return Preview{}, err
	}
	e := parseError(answer)
	switch {
	case status == http.StatusCreated:
	case known(e.Code):
		return Preview{}, refusal(e, e.Code, "the platform refused the preview of "+id)
	case status == http.StatusNotFound:
		return Preview{}, refusal(e, "E-PUB-013", "the platform knows no publish "+id)
	case status == http.StatusForbidden:
		return Preview{}, refusal(e, "E-PUB-005", "the publish "+id+" is of another builder's app")
	default:
		return Preview{}, unexpected(status, answer)
	}
	var p Preview
	if json.Unmarshal(answer, &p) != nil || !p.valid(id) {
		return Preview{}, errors.New("the platform's answer about the preview of " + id + " is not valid")
	}
	p.Started, p.Expires = p.Started.UTC(), p.Expires.UTC()
	return p, nil
}

// valid reports whether p is a preview of the publish id whose names and address are well formed.
func (p *Preview) valid(id string) bool {
	u, err := url.Parse(p.URL)
	return p.Publish == id && manifest.ValidApp(p.App) && len(p.Host) <= 253 && hostName.MatchString(p.Host) &&
		err == nil && u.Scheme == "https" && u.User == nil && u.Host == p.Host && visible(p.URL, 2048) &&
		!p.Started.IsZero() && p.Expires.After(p.Started)
}

// PreviewEnd ends the preview of the app on the platform, which then deletes its data. When no
// preview of the app runs, it says so. An app the platform does not know is E-PUB-019, and one
// the builder does not own E-PUB-005, as the platform's answer says.
func (c *Client) PreviewEnd(ctx context.Context, app string) error {
	body, err := json.Marshal(map[string]string{"app": app})
	if err != nil {
		return err
	}
	status, answer, err := c.call(ctx, c.HTTP, http.MethodPost, "/v1/previews/end", "application/json", body, maxAnswer)
	if err != nil {
		return err
	}
	e := parseError(answer)
	switch {
	case status == http.StatusNoContent:
		return nil
	case known(e.Code):
		return refusal(e, e.Code, "the platform refused to end the preview of "+app)
	case status == http.StatusNotFound && e.Error == "not_found":
		return errors.New("no preview of " + app + " is running")
	case status == http.StatusForbidden:
		return refusal(e, "E-PUB-005", "the app "+app+" belongs to another builder")
	}
	return unexpected(status, answer)
}
