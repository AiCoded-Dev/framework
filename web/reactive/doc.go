// Package reactive carries live page values between the server and the browser over one
// WebSocket per page. The web package opens and runs the connection, and generated code sends
// values through it; app code uses [Topic] and [Broadcast].
//
// A template declares a live value with <ssr:var name="lastSeen" type="string" reactive="true"/>.
// Data sets its first value, and the page's Subscribe hook keeps it current while the page is
// open. Subscribe usually waits on a [Topic] or a [Broadcast] that the code which changes the
// value publishes to, and returns when its context ends:
//
//	func (p *DP) Subscribe(ctx context.Context, r *web.Request, state *ReactiveState) error {
//		sub := p.d.LastSeen.Subscribe(r.URLParam("login"))
//		defer sub.Close()
//		for {
//			select {
//			case <-ctx.Done():
//				return nil
//			case v := <-sub.Updates():
//				state.SetLastSeen(v)
//			}
//		}
//	}
//
// With client-writable="true" the page writes the value too, through ssr:bind or ssr.set, and
// Validate<Name> checks every write: its web.Error message reaches the page's ssr.onError.
//
// A live value in {{ }} reaches the page as text, never as markup. A viewer has at most 8 live
// connections at a time. A connection passes the page's access rules and Guards when it opens,
// and opens again at least every 10 minutes.
//
// Read more in the guide docs/guides/live-values.md and the task docs/tasks/add-live-value.md,
// which aicoded explain and the MCP tool howto print as guides/live-values and
// tasks/add-live-value.
package reactive
