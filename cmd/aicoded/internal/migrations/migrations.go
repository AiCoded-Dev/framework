// Package migrations gathers the steps that aicoded dev and aicoded check apply to an app's
// database on the builder's computer: the migrations the app published, fetched from the platform
// and cached in .aicoded/migrations.json, then the statements of its draft, .aicoded/migration.sql,
// as steps of the publish "draft". It also clears the draft once a publish that carried it passed.
// The cache holds steps and the publish that carried the draft, never a token.
package migrations

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"strings"
	"time"

	"aicoded.dev/framework/cmd/aicoded/internal/draft"
	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/manifest"
	"aicoded.dev/framework/migrate"
)

const (
	// Cache is where an app keeps the migrations it published, relative to its folder.
	Cache = ".aicoded/migrations.json"
	// Draft is the publish of the steps of the draft.
	Draft = "draft"

	// fetchTimeout bounds asking the platform for the published migrations.
	fetchTimeout = 10 * time.Second
	// maxCache is the most bytes of the cache read.
	maxCache  = 8 << 20
	cacheTemp = ".aicoded/.migrations.json.tmp"
)

// Platform returns the steps of the migrations that an app published, as the platform's client
// does. A Platform that has nothing newer than the cache to give returns ErrUseCache.
type Platform interface {
	Migrations(ctx context.Context, app string) ([]migrate.Step, error)
}

// ErrUseCache is the error of a Platform whose answer is the cache.
var ErrUseCache = errors.New("use the cache")

// cache is the content of .aicoded/migrations.json.
type cache struct {
	Steps []migrate.Step `json:"steps"`
	// Sent is the publish that carried the draft, until its outcome is known.
	Sent *sent `json:"sent,omitempty"`
}

// sent is a publish that carried the draft, and the hash of the draft's text it carried.
type sent struct {
	Publish string `json:"publish"`
	SHA256  string `json:"sha256"`
}

// Steps returns the steps to apply to a database of the app in dir whose permission list is m:
// those it published, then its draft's. The published ones come from p, and are cached; when p is
// nil, from the cache. When p cannot give them, they come from the cache, or there are none, and
// warning says so in one line. A draft that a publish carried, unchanged, adds no step once that
// publish's steps are among the published ones, before Settle clears it. Its error is that of
// draft.Check.
func Steps(ctx context.Context, dir string, m manifest.Manifest, p Platform) (steps []migrate.Step, warning string, err error) {
	stmts, _, err := draft.Check(dir, m)
	if err != nil {
		return nil, "", err
	}
	published, warning := Published(ctx, dir, m.App, p)
	if publishedDraft(dir, published) {
		stmts = nil
	}
	return Append(published, Draft, stmts), warning, nil
}

// publishedDraft reports whether the draft of the app in dir is the one the publish the cache
// waits for carried, and published holds that publish's steps.
func publishedDraft(dir string, published []migrate.Step) bool {
	c, _, err := read(dir)
	if err != nil || c.Sent == nil || !slices.ContainsFunc(published, func(s migrate.Step) bool { return s.Publish == c.Sent.Publish }) {
		return false
	}
	text, err := draft.Read(dir)
	return err == nil && text != nil && hash(text) == c.Sent.SHA256
}

// Published returns the steps of the migrations that the app app in dir published, as Steps
// does. When the platform lists none but the cache holds some, or the answer is the cache and it
// cannot be read, warning says so. When p is not signed in, E-CLI-004, warning starts with that
// error rather than with the platform it could not ask.
func Published(ctx context.Context, dir, app string, p Platform) (steps []migrate.Step, warning string) {
	c, found, cerr := read(dir)
	cached := func() ([]migrate.Step, string) {
		if cerr != nil {
			return nil, cerr.Error() + ", so only the draft is applied"
		}
		return c.Steps, ""
	}
	if p == nil {
		return cached()
	}
	fctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	steps, err := p.Migrations(fctx, app)
	cancel()
	if errors.Is(err, ErrUseCache) {
		return cached()
	}
	if err != nil {
		why, theCached := "could not get the migrations "+app+" published from the platform: "+reason(err), "the ones"
		if errs.Code(err) == "E-CLI-004" {
			why, theCached = reason(err), "the migrations"
		}
		switch {
		case cerr != nil:
			return nil, why + "; " + cerr.Error() + ", so only the draft is applied"
		case found:
			return c.Steps, why + "; " + theCached + " cached in " + Cache + " are applied"
		}
		return nil, why + "; only the draft is applied"
	}
	if len(steps) == 0 && len(c.Steps) > 0 {
		warning = fmt.Sprintf("the platform lists no migrations of %s, but %s holds %d steps: check that AICODED_PLATFORM names the "+
			"platform you publish to; the platform's answer is applied", app, Cache, len(c.Steps))
	}
	if found && !slices.Equal(steps, c.Steps) || !found && (len(steps) > 0 || cerr != nil) {
		c.Steps = steps
		_ = write(dir, c)
	}
	return steps, warning
}

// Append returns steps followed by stmts, as the steps of the publish pub, numbered on.
func Append(steps []migrate.Step, pub string, stmts []string) []migrate.Step {
	out := make([]migrate.Step, 0, len(steps)+len(stmts))
	out = append(out, steps...)
	for i, s := range stmts {
		out = append(out, migrate.Step{Seq: len(steps) + i + 1, Publish: pub, Index: i + 1, SHA256: hash([]byte(s)), Statement: s})
	}
	return out
}

// MarkSent keeps in the cache of the app in dir that the publish id carried text, the draft.
func MarkSent(dir, id string, text []byte) error {
	c, _, err := read(dir)
	if err != nil {
		c = cache{}
	}
	c.Sent = &sent{Publish: id, SHA256: hash(text)}
	return write(dir, c)
}

// Pending returns the publish that carried the draft of the app in dir and has no outcome yet,
// as far as the cache knows, or "".
func Pending(dir string) string {
	c, _, err := read(dir)
	if err != nil || c.Sent == nil {
		return ""
	}
	return c.Sent.Publish
}

// Forget makes the cache of the app in dir stop waiting for the publish id, which the platform
// does not know, and leaves the draft as it is.
func Forget(dir, id string) error {
	c, _, err := read(dir)
	if err != nil || c.Sent == nil || c.Sent.Publish != id {
		return nil
	}
	c.Sent = nil
	return write(dir, c)
}

// Settle ends the wait of the cache of the app app in dir for the publish id, which ended with
// status. When the publish carried the draft and passed, Settle deletes the draft, unless it
// changed since, and caches the published migrations from p. Whatever the outcome, the cache
// forgets the publish. It returns a line for the builder about the draft, or "" when the
// publish did not carry it.
func Settle(ctx context.Context, dir, app, id, status string, p Platform) (string, error) {
	c, _, err := read(dir)
	if err != nil || c.Sent == nil || c.Sent.Publish != id {
		return "", nil
	}
	carried := c.Sent.SHA256
	c.Sent = nil
	if status != "passed" {
		return fmt.Sprintf("publish %s %s, so %s stays as it was", id, status, draft.File), write(dir, c)
	}
	note := fmt.Sprintf("publish %s passed with the schema change in %s, which is cleared", id, draft.File)
	text, err := draft.Read(dir)
	switch {
	case err == nil && text != nil && hash(text) != carried:
		note = fmt.Sprintf("publish %s passed with the schema change that %s held when it was sent; the draft changed since, so it stays: "+
			"remove from it the statements that publish carried", id, draft.File)
	case err == nil && text != nil:
		err = remove(dir)
	}
	if err != nil {
		return "", err
	}
	if p != nil {
		fctx, cancel := context.WithTimeout(ctx, fetchTimeout)
		if steps, ferr := p.Migrations(fctx, app); ferr == nil {
			c.Steps = steps
		}
		cancel()
	}
	return note, write(dir, c)
}

// read returns the cache of the app in dir, and whether there is one. A cache that is not a
// regular file inside dir, or that cannot be read or parsed, is an error.
func read(dir string) (cache, bool, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return cache{}, false, unreadable(err)
	}
	defer root.Close()
	f, err := root.Open(Cache)
	if errors.Is(err, fs.ErrNotExist) {
		return cache{}, false, nil
	}
	if err != nil {
		return cache{}, false, unreadable(err)
	}
	defer f.Close()
	if info, err := f.Stat(); err != nil || !info.Mode().IsRegular() {
		return cache{}, false, unreadable(errors.New("not a regular file"))
	}
	data, err := io.ReadAll(io.LimitReader(f, maxCache+1))
	if err != nil {
		return cache{}, false, unreadable(err)
	}
	var c cache
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if len(data) > maxCache || dec.Decode(&c) != nil {
		return cache{}, false, errors.New("the cache " + Cache + " is not valid")
	}
	return c, true, nil
}

// write replaces the cache of the app in dir with c, inside dir.
func write(dir string, c cache) error {
	if c.Steps == nil {
		c.Steps = []migrate.Step{}
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.MkdirAll(".aicoded", 0o755); err != nil {
		return err
	}
	if err := root.WriteFile(cacheTemp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return root.Rename(cacheTemp, Cache)
}

// remove deletes the draft of the app in dir.
func remove(dir string) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	return root.Remove(draft.File)
}

// unreadable is the error of a cache that cannot be read.
func unreadable(err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	return errors.New("the cache " + Cache + " cannot be read: " + err.Error())
}

// reason is the message of err on one line, without its fix.
func reason(err error) string {
	var coded *errs.Error
	if errors.As(err, &coded) {
		return coded.Code + ": " + coded.Msg
	}
	return strings.Join(strings.Fields(err.Error()), " ")
}

// hash returns the lower-case hex SHA-256 of b.
func hash(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
