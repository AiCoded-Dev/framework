package main

import (
	"fmt"
	"io"
	"net/http"

	"aicoded.dev/framework/outbound"
	"aicoded.dev/framework/secrets"
)

// call calls the outside service at api.example.com and writes its status and body, or the error.
func call(w http.ResponseWriter, r *http.Request) {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://api.example.com/echo", http.NoBody)
	if err != nil {
		fmt.Fprint(w, err)
		return
	}
	resp, err := outbound.Client().Do(req)
	if err != nil {
		fmt.Fprint(w, err)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprint(w, err)
		return
	}
	fmt.Fprintf(w, "%d %s", resp.StatusCode, body)
}

// bound writes the error of reading partner_key, which the tests bind to api.example.com.
func bound(w http.ResponseWriter, r *http.Request) {
	_, err := secrets.Get(r.Context(), "partner_key")
	fmt.Fprint(w, err)
}
