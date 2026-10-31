// Package good calls outside services the way app code may.
package good

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"aicoded.dev/framework/outbound"
)

// deps holds the client the app shares.
type deps struct{ client *http.Client }

// Fetch reads a page of an outside service through the runner.
func Fetch(ctx context.Context, u string) (string, error) {
	c := outbound.Client()
	var held *http.Client
	held = c
	d := deps{client: held}
	_ = fmt.Sprint(d)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return "", err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	return read(resp)
}

// Post sends a form to an outside service through the runner.
func Post(ctx context.Context, c *http.Client, u string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, u, strings.NewReader("a=1"))
	if err != nil {
		return nil, err
	}
	defer c.CloseIdleConnections()
	return c.Do(req.WithContext(ctx))
}

func read(resp *http.Response) (string, error) {
	b, err := io.ReadAll(resp.Body)
	return string(b), err
}
