package reactive

import (
	"encoding/json"

	"github.com/coder/websocket"
)

// Kinds of Binding.
const (
	KindText = "text" // shown with textContent
	KindHTML = "html" // markup rendered by the page's escapers
)

// Codes of err frames.
const (
	CodeUnknownRoute = "unknown_route"
	CodeValidation   = "validation_failed"
	CodeDecode       = "decode_error"
)

// StatusReauth closes a connection that reached its maximum age; the browser reconnects at
// once, so access is checked again.
const StatusReauth websocket.StatusCode = 4000

// Binding is the current value of one live site on a page.
type Binding struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// TextBinding returns a binding the browser shows as plain text.
//
// Generated code only.
func TextBinding(text string) Binding { return Binding{Kind: KindText, Value: text} }

// HTMLBinding returns a binding holding markup the page's escapers produced.
//
// Generated code only.
func HTMLBinding(markup string) Binding { return Binding{Kind: KindHTML, Value: markup} }

// InitMsg carries every live value of the page when a connection opens.
type InitMsg struct {
	T        string             `json:"t"`
	Bindings map[string]Binding `json:"bindings"`
}

// PatchMsg carries a changed live value.
type PatchMsg struct {
	T   string `json:"t"`
	Key string `json:"key"`
	Binding
}

// WriteMsg is a value the page sends for a client-writable variable.
type WriteMsg struct {
	T        string          `json:"t"`
	RouteKey string          `json:"routeKey"`
	Var      string          `json:"var"`
	Value    json.RawMessage `json:"value"`
}

// AckMsg accepts a write.
type AckMsg struct {
	T        string `json:"t"`
	RouteKey string `json:"routeKey"`
	Var      string `json:"var"`
}

// ErrMsg refuses a write.
type ErrMsg struct {
	T        string `json:"t"`
	RouteKey string `json:"routeKey"`
	Var      string `json:"var"`
	Msg      string `json:"msg"`
	Code     string `json:"code"`
}

// NewInit returns the init frame.
func NewInit(bindings map[string]Binding) InitMsg { return InitMsg{T: "init", Bindings: bindings} }

// NewErr returns an err frame.
func NewErr(routeKey, varName, text, code string) ErrMsg {
	return ErrMsg{T: "err", RouteKey: routeKey, Var: varName, Msg: text, Code: code}
}

// CallMsg asks the server to run one of a page's calls.
type CallMsg struct {
	T        string          `json:"t"`
	ID       uint32          `json:"id"`
	RouteKey string          `json:"routeKey"`
	Name     string          `json:"name"`
	Args     json.RawMessage `json:"args"`
}

// ResultMsg answers a call.
type ResultMsg struct {
	T     string          `json:"t"`
	ID    uint32          `json:"id"`
	Value json.RawMessage `json:"value"`
}

// FailMsg answers a call that failed.
type FailMsg struct {
	T      string `json:"t"`
	ID     uint32 `json:"id"`
	Status int    `json:"status"`
	Msg    string `json:"msg"`
}

// NewResult returns a result frame; value is already JSON.
func NewResult(id uint32, value json.RawMessage) ResultMsg {
	return ResultMsg{T: "result", ID: id, Value: value}
}

// NewFail returns a fail frame.
func NewFail(id uint32, status int, text string) FailMsg {
	return FailMsg{T: "fail", ID: id, Status: status, Msg: text}
}
