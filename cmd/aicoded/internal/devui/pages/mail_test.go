package pages_test

import (
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/devui/fake"
)

// mailed is a workspace of shop and billing, where shop sent a message whose every part holds
// markup and got one without a subject, text or HTML.
func mailed() *fake.Backend {
	at := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	return &fake.Backend{Data: fake.Data{
		Status: devapi.Status{Apps: []devapi.App{{Name: "shop"}, {Name: "billing"}}},
		Mail: map[string][]devapi.Mail{"shop": {
			{
				MailSummary: devapi.MailSummary{
					ID: "sent-1", Folder: "sent", From: "shop@acme.example", To: []string{"<b>ana</b>@acme.example", "bob@acme.example"},
					Subject: "<script>alert(1)</script>", Time: at,
				},
				Cc: []string{"<i>cc</i>@acme.example"}, ReplyTo: "help@acme.example", Text: "Hello <b>Ana</b>",
				HTML:        `<p>Hi</p><script>parent.document.title = "stolen"</script>`,
				Attachments: []devapi.Attachment{{Name: "<i>bill</i>.pdf", Type: "application/pdf", Size: 1024}},
			},
			{MailSummary: devapi.MailSummary{ID: "inbox-2", Folder: "inbox", From: "bob@acme.example", To: []string{}, Time: at}},
		}},
	}}
}

func TestMailPages(t *testing.T) {
	srv := serve(t, mailed())
	status, page := get(t, srv, "/mail")
	require.Equal(t, http.StatusOK, status)
	assert.Contains(t, page, `<li><a href="/mail/shop">shop</a></li><li><a href="/mail/billing">billing</a></li>`)

	status, page = get(t, srv, "/mail/shop")
	require.Equal(t, http.StatusOK, status)
	assert.Contains(t, page, `<td>sent</td>`)
	assert.Contains(t, page, `<td>shop@acme.example</td><td>&lt;b&gt;ana&lt;/b&gt;@acme.example, bob@acme.example</td>`)
	assert.Contains(t, page, `<a href="/mail/shop/sent-1">&lt;script&gt;alert(1)&lt;/script&gt;</a>`)
	assert.Contains(t, page, `<a href="/mail/shop/inbox-2">(no subject)</a>`)
	assert.NotContains(t, page, "<script>")
	assert.NotContains(t, page, "<b>")

	status, _ = get(t, srv, "/mail/ledger")
	assert.Equal(t, http.StatusNotFound, status)
}

// A post to the mail of an app that the workspace does not have is 404 and reaches no mailbox.
func TestMailPageUnknownApp(t *testing.T) {
	b := mailed()
	status, _ := postFrom(t, serve(t, b), "/mail/shop", "/mail/ledger", "receive", url.Values{"from": {"cy@acme.example"}})
	assert.Equal(t, http.StatusNotFound, status)
	assert.Empty(t, b.Calls("MailReceive"))
	assert.Equal(t, []fake.Call{{Method: "Mail", App: "shop"}}, b.Calls("Mail"), "only the page the form came from")
}

func TestMailPageFollowsTheWorkspace(t *testing.T) {
	b := mailed()
	srv := serve(t, b)
	_, page := get(t, srv, "/mail/shop")
	key := liveKey(t, page, "messages")
	c := live(t, srv, "/mail/shop")
	assert.Contains(t, nextPatch(t, c, key), "sent-1")

	_, err := b.MailReceive(t.Context(), "shop", devapi.InboundMail{From: "cy@acme.example", Subject: "<i>new</i>"})
	require.NoError(t, err)
	assert.Contains(t, nextPatch(t, c, key), `<a href="/mail/shop/inbox-3">&lt;i&gt;new&lt;/i&gt;</a>`)
}

func TestReceiveMail(t *testing.T) {
	b := mailed()
	srv := serve(t, b)
	status, _ := post(t, srv, "/mail/shop", "receive", url.Values{"from": {"cy@acme.example"}, "subject": {"Hi"}, "text": {"Hello"}})
	assert.Equal(t, http.StatusSeeOther, status)
	assert.Equal(t, []fake.Call{{
		Method: "MailReceive", App: "shop", Arg: devapi.InboundMail{From: "cy@acme.example", Subject: "Hi", Text: "Hello"},
	}}, b.Calls("MailReceive"))

	status, page := post(t, srv, "/mail/shop", "receive", url.Values{"subject": {"Hi"}})
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, page, "Give the address the mail comes from.")
	assert.Len(t, b.Calls("MailReceive"), 1, "a form without a sender reaches no inbox")
}

func TestReceiveMailShowsWhyItFailed(t *testing.T) {
	b := mailed()
	b.Data.Errors = map[string]error{"MailReceive": errors.New("shop declares no <b>email</b> in aicoded.yaml")}
	status, page := post(t, serve(t, b), "/mail/shop", "receive", url.Values{"from": {`"><b>cy</b>@acme.example`}})
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, page, `<p class="error" id="receive-error">shop declares no &lt;b&gt;email&lt;/b&gt; in aicoded.yaml</p>`)
	assert.Contains(t, page, `value="&#34;&gt;&lt;b&gt;cy&lt;/b&gt;@acme.example"`, "the form keeps what was sent")
	assert.NotContains(t, page, "<b>")
}

func TestMessagePage(t *testing.T) {
	srv := serve(t, mailed())
	status, page := get(t, srv, "/mail/shop/sent-1")
	require.Equal(t, http.StatusOK, status)
	assert.Contains(t, page, "<h1>&lt;script&gt;alert(1)&lt;/script&gt;</h1>")
	assert.Contains(t, page, "<th>Cc</th><td>&lt;i&gt;cc&lt;/i&gt;@acme.example</td>")
	assert.Contains(t, page, "<th>Reply-To</th><td>help@acme.example</td>")
	assert.NotContains(t, page, "<th>Bcc</th>")
	assert.Contains(t, page, `<pre id="text">Hello &lt;b&gt;Ana&lt;/b&gt;</pre>`)
	assert.Contains(t, page, `<iframe sandbox src="/_aicoded/mail/shop/sent-1" title="The HTML part of the message"></iframe>`)
	assert.NotContains(t, page, "stolen", "the HTML part is only in the frame")
	assert.Contains(t, page, "<td>&lt;i&gt;bill&lt;/i&gt;.pdf</td><td>application/pdf</td><td>1024 bytes</td>")
	assert.NotContains(t, page, "<script>")
	assert.NotContains(t, page, "<b>")

	status, page = get(t, srv, "/mail/shop/inbox-2")
	require.Equal(t, http.StatusOK, status)
	assert.Contains(t, page, "<h1>(no subject)</h1>")
	assert.Contains(t, page, "The message has no text part.")
	assert.NotContains(t, page, "<iframe")

	for _, path := range []string{"/mail/shop/sent-9", "/mail/ledger/sent-1"} {
		status, _ = get(t, srv, path)
		assert.Equal(t, http.StatusNotFound, status, path)
	}
}
