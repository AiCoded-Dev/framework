// Command desk is the app the dev UI's end-to-end tests run. It logs, traces and mails on
// request, and puts its secret token in all three, which aicoded dev must hide.
package main

import (
	"crypto/rand"
	"fmt"
	"log/slog"
	"net/http"

	"aicoded.dev/framework/app"
	"aicoded.dev/framework/mailer"
	"aicoded.dev/framework/secrets"
	"aicoded.dev/framework/telemetry"
)

// page is the HTML part of the mail: green, unless its script or its image's error handler runs
// and turns it red.
const page = `<body style="margin:0;background:#00ff00"><script>document.body.style.background = "#ff0000"</script>` +
	`<img src="x" onerror="document.body.style.background = '#ff0000'"><!-- %s --></body>`

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, "desk") })
	mux.HandleFunc("GET /say", say)
	mux.HandleFunc("GET /mail", mail)
	app.Main(app.Options{Handler: mux})
}

// say logs the msg of the query with the secret token, in a span that holds the token too.
func say(w http.ResponseWriter, r *http.Request) {
	ctx, span := telemetry.Start(r.Context(), "say")
	defer span.End()
	token, err := secrets.Get(ctx, "token")
	if err != nil {
		http.Error(w, "no token", http.StatusInternalServerError)
		return
	}
	span.SetAttr("token", token.Reveal())
	slog.InfoContext(ctx, r.URL.Query().Get("msg"), "token", token.Reveal())
	fmt.Fprint(w, "said")
}

// mail sends a mail whose HTML part is page, with the secret token in its text and its HTML.
func mail(w http.ResponseWriter, r *http.Request) {
	token, err := secrets.Get(r.Context(), "token")
	if err != nil {
		http.Error(w, "no token", http.StatusInternalServerError)
		return
	}
	_, err = mailer.Send(r.Context(), mailer.Message{
		IdempotencyKey: rand.Text(),
		To:             []mailer.Address{{Address: "team@desk.test"}},
		Subject:        "Scripted",
		Text:           "The token is " + token.Reveal(),
		HTML:           fmt.Sprintf(page, token.Reveal()),
	})
	if err != nil {
		http.Error(w, "not sent", http.StatusInternalServerError)
		return
	}
	fmt.Fprint(w, "sent")
}
