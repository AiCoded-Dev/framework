package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"

	"aicoded.dev/framework/migrate"
)

// maxMigrations is the largest answer about an app's migrations read from the platform: 8 MiB,
// room for the JSON of a list of 2000 steps and 1 MiB of statements.
const maxMigrations = 8 << 20

// maxSteps is the most steps the platform's list of an app's migrations holds.
const maxSteps = 2000

// Migrations returns the steps of the migrations that the app name published, in order, as the
// platform applies them when a release rolls out. An app the platform does not have has none. Another
// builder's app is E-PUB-005. Every step must be numbered in order, come from a publish and hold
// the hash of its statement.
func (c *Client) Migrations(ctx context.Context, name string) ([]migrate.Step, error) {
	body, err := json.Marshal(map[string]string{"app": name})
	if err != nil {
		return nil, err
	}
	status, answer, err := c.call(ctx, c.HTTP, http.MethodPost, "/v1/apps/migrations", "application/json", body, maxMigrations)
	if err != nil {
		return nil, err
	}
	e := parseError(answer)
	switch {
	case status == http.StatusOK:
	case status == http.StatusNotFound && e.Error == "not_found":
		return nil, nil
	case pubCode.MatchString(e.Code):
		return nil, refusal(e, e.Code, "the platform refused the migrations of "+name)
	case status == http.StatusForbidden && e.Error == "insufficient_scope":
		return nil, refusal(e, "E-PUB-005", "the app "+name+" belongs to another builder")
	default:
		return nil, unexpected(status, answer)
	}
	var a struct {
		Steps []migrate.Step `json:"steps"`
	}
	if json.Unmarshal(answer, &a) != nil || !validSteps(a.Steps) {
		return nil, errors.New("the platform's answer about the migrations of " + name + " is not valid")
	}
	return a.Steps, nil
}

// validSteps reports whether steps are at most 2000, numbered from 1, each of a publish, with
// its place in the publish and the hash of its statement.
func validSteps(steps []migrate.Step) bool {
	if len(steps) > maxSteps {
		return false
	}
	for i, s := range steps {
		sum := sha256.Sum256([]byte(s.Statement))
		if s.Seq != i+1 || !PublishID.MatchString(s.Publish) || s.Index < 1 || s.Statement == "" || s.SHA256 != hex.EncodeToString(sum[:]) {
			return false
		}
	}
	return true
}
