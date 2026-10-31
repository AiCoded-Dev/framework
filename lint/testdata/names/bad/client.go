package bad

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"aicoded.dev/framework/outbound"
)

// valueClient holds a client of its own.
type valueClient struct{ c http.Client }

// ownClient and aliasClient are http.Client under the app's own names.
type (
	ownClient   http.Client
	aliasClient = http.Client
)

// Clients makes clients of its own and changes the one outbound.Client returns.
func Clients(ctx context.Context, req *http.Request) {
	_ = http.Client{}
	_ = &http.Client{}
	_ = new(http.Client)
	var zero http.Client
	_ = []*http.Client{{}}
	_ = map[string]*http.Client{"a": {}}
	_ = &http.Transport{}
	_ = new(http.Transport)
	_ = http.DefaultClient
	_ = http.DefaultTransport
	_, _ = http.Get("https://api.partner.example/")
	_, _ = http.Post("https://api.partner.example/", "text/plain", nil)
	_, _ = http.Head("https://api.partner.example/")
	_, _ = http.PostForm("https://api.partner.example/", nil)
	c := outbound.Client()
	c.Transport = nil
	c.CheckRedirect = nil
	c.Jar = nil
	c.Timeout = 0
	*c = zero
	_, _ = c.Get("https://api.partner.example/")
	_ = json.Unmarshal([]byte(`{"Transport":null}`), c)
	fmt.Println(holder{c})
	_ = zeroOf(c)
}

// holder carries a client, which reflection could reach through an interface value.
type holder struct{ C *http.Client }

func zeroOf[T any](*T) (zero T) { return zero }

// wrapped embeds the client, so its fields and methods are promoted.
type wrapped struct{ *http.Client }

// Reach reaches the client through embedding, interface values and type arguments.
func Reach(c *http.Client, data []byte) any {
	w := wrapped{c}
	w.Transport = nil
	_, _ = w.Head("https://api.partner.example/")
	var v any = c
	_ = json.Unmarshal(data, &c)
	_ = pass[*http.Client](c)
	_ = v
	return w
}

func pass[T any](v T) T { return v }
