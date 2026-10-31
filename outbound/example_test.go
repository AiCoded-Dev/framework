package outbound_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"aicoded.dev/framework/outbound"
)

// Deps is what an app's pages share, as deps.Deps is in an app. It keeps the client in an
// unexported field: aicoded check refuses a value that holds a client in an exported field when
// it becomes an interface value, as each page's data provider does in the generated code
// (E-LINT-009).
type Deps struct {
	weather *http.Client
}

// New returns Deps with the client whose requests go through the runner.
func New() *Deps {
	return &Deps{weather: outbound.Client()}
}

// Today returns the weather service's summary of today in city. ctx is the context of the
// request or job it serves. The service's host, api.weather.example, is in egress in the
// permission list, and the runner adds the key that egress binds to it.
func (d *Deps) Today(ctx context.Context, city string) (string, error) {
	u := "https://api.weather.example/v1/today?city=" + url.QueryEscape(city)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return "", err
	}
	resp, err := d.weather.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("the weather service answered %s", resp.Status)
	}
	var today struct{ Summary string }
	if err := json.NewDecoder(resp.Body).Decode(&today); err != nil {
		return "", err
	}
	return today.Summary, nil
}

// Call an outside service that egress in the permission list names, from a method of the app's
// Deps, with the context of the request or job the call serves. The example only compiles: it
// needs the runner of a running app.
func ExampleClient() {
	_ = New().Today
}
