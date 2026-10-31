package outbound_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"aicoded.dev/framework/outbound"
)

// Call an outside service that egress in the permission list names, here in a page's Data, with
// the context Data receives. The runner adds the key that egress binds to the host. The example
// only compiles: it needs the runner of a running app.
func ExampleClient() {
	data := func(ctx context.Context, city string) error {
		u := "https://api.weather.example/v1/today?city=" + url.QueryEscape(city)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
		if err != nil {
			return err
		}
		resp, err := outbound.Client().Do(req)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("weather service answered %s", resp.Status)
		}
		var today struct{ Summary string }
		if err := json.NewDecoder(resp.Body).Decode(&today); err != nil {
			return err
		}
		fmt.Println(today.Summary)
		return nil
	}
	_ = data
}
