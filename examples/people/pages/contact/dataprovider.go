package contact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/mail"
	"strings"
	"unicode/utf8"

	"aicoded.dev/framework/auth"
	"aicoded.dev/framework/mailer"
	"aicoded.dev/framework/web"
	"aicoded.dev/framework/web/form"

	"people/deps"
)

var _ RouteDataProvider = &DP{}

// DP provides the contact form.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Data shows the banner after a message was sent.
func (p *DP) Data(_ context.Context, r *web.Request, _ web.ResponseWriter, data *RouteData) error {
	data.Sent = r.URL.Query().Get("sent") == "1"
	return nil
}

// InitContact offers the topics.
func (p *DP) InitContact(_ context.Context, _ *web.Request, _ web.ResponseWriter, f *FormContactValues) error {
	f.Topic.SetOptions([]form.SelectOptionElement[string]{
		form.SelectOption[string]{Value: "General question", Label: "General question"},
		form.SelectOption[string]{Value: "Technical support", Label: "Technical support"},
		form.SelectOption[string]{Value: "Billing", Label: "Billing"},
		form.SelectOption[string]{Value: "Feedback", Label: "Feedback"},
	})
	return nil
}

// ProcessContact mails the message to the team, then shows the banner. It stores and logs
// nothing. The same viewer sending the same message again sends no second mail.
func (p *DP) ProcessContact(ctx context.Context, _ *web.Request, _ web.ResponseWriter, f *FormContactValues) error {
	name := strings.TrimSpace(f.Name.GetValue())
	email := strings.TrimSpace(f.Email.GetValue())
	message := strings.TrimSpace(f.Message.GetValue())
	switch {
	case name == "":
		f.Name.SetError("Enter your name.")
	case utf8.RuneCountInString(name) > 100:
		f.Name.SetError("Use at most 100 characters.")
	}
	switch {
	case !plainAddress(email):
		f.Email.SetError("Enter an email address.")
	case len(email) > 254:
		f.Email.SetError("Use at most 254 characters.")
	}
	switch {
	case message == "":
		f.Message.SetError("Enter a message.")
	case utf8.RuneCountInString(message) > 2000:
		f.Message.SetError("Use at most 2000 characters.")
	}
	if f.HasError() {
		return nil
	}
	topic := f.Topic.GetValue()
	_, err := mailer.Send(ctx, mailer.Message{
		IdempotencyKey: contactKey(auth.Viewer(ctx).Subject, topic, message),
		To:             []mailer.Address{{Address: p.d.TeamAddress}},
		Subject:        "Contact form: " + topic,
		Text:           fmt.Sprintf("From: %s <%s>\nTopic: %s\n\n%s\n", name, email, topic, message),
	})
	if err != nil {
		return err
	}
	return web.Redirect("/contact?sent=1")
}

// plainAddress reports whether s is an email address and nothing else: no display name, angle
// brackets or comment, which would reach the mail's text.
func plainAddress(s string) bool {
	a, err := mail.ParseAddress(s)
	return err == nil && a.Name == "" && a.Address == s
}

// contactKey names a message by its sender, topic and text, so that a message sent twice is
// mailed once.
func contactKey(subject, topic, message string) string {
	sum := sha256.Sum256([]byte(subject + "\x00" + topic + "\x00" + message))
	return "contact-" + hex.EncodeToString(sum[:16])
}
