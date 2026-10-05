package mailrules

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/runnerproto/runnerv1"
)

type req = runnerv1.SendRequest

func ok() *req {
	return &req{
		IdempotencyKey: "invoice-42",
		FromName:       "Ops Team",
		To:             []*runnerv1.Address{{Name: "Ana Lúcia", Address: "ana@acme.example"}},
		Subject:        "Invoice 42",
		Text:           "Hello",
		Headers:        []*runnerv1.Header{{Name: "In-Reply-To", Value: "<a@acme.example>"}},
		Attachments:    []*runnerv1.Attachment{{Filename: "42.pdf", ContentType: "application/pdf", Data: []byte("%PDF")}},
	}
}

func TestCheck(t *testing.T) {
	require.NoError(t, Check(ok()))
	to := func(a string) func(*req) { return func(m *req) { m.To[0].Address = a } }
	subject := func(s string) func(*req) { return func(m *req) { m.Subject = s } }
	file := func(f string) func(*req) { return func(m *req) { m.Attachments[0].Filename = f } }
	ctype := func(s string) func(*req) { return func(m *req) { m.Attachments[0].ContentType = s } }
	// fill sizes the attachment so that the message is MaxSize+extra bytes.
	fill := func(extra int) func(*req) {
		return func(m *req) {
			m.Attachments[0].Data = make([]byte, MaxSize+extra-len(m.Subject)-len(m.Text)-len(m.Attachments[0].ContentType))
		}
	}
	for name, c := range map[string]struct {
		change func(*req)
		code   string
	}{
		"no key":             {func(m *req) { m.IdempotencyKey = "" }, "E-MAIL-001"},
		"key with a space":   {func(m *req) { m.IdempotencyKey = "a b" }, "E-MAIL-001"},
		"long key":           {func(m *req) { m.IdempotencyKey = strings.Repeat("k", 129) }, "E-MAIL-001"},
		"no recipients":      {func(m *req) { m.To = nil }, "E-MAIL-002"},
		"display name trick": {to("Ana <ana@acme.example>"), "E-MAIL-002"},
		"no domain dot":      {to("ana@localhost"), "E-MAIL-002"},
		"IDN":                {to("ana@bücher.example"), "E-MAIL-002"},
		"CRLF in address":    {func(m *req) { m.Cc = []*runnerv1.Address{{Address: "a@b.example\r\nBcc: x@evil.example"}} }, "E-MAIL-002"},
		"CRLF in both": {func(m *req) {
			m.To[0] = &runnerv1.Address{Name: "Ana\r\n", Address: "a@b.example\r\nBcc: x@evil.example"}
		}, "E-MAIL-002"},
		"huge address":        {to(strings.Repeat("a", 1<<20) + "@acme.example"), "E-MAIL-002"},
		"domain literal":      {to("a@[127.0.0.1]"), "E-MAIL-002"},
		"IP address":          {to("a@1.2.3.4"), "E-MAIL-002"},
		"hyphen first":        {to("a@-x.example"), "E-MAIL-002"},
		"hyphen label":        {to("a@x.-y.example"), "E-MAIL-002"},
		"underscore":          {to("a@x_y.example"), "E-MAIL-002"},
		"upper-case domain":   {to("Ana@ACME.Example"), ""},
		"bad Reply-To":        {func(m *req) { m.ReplyTo = &runnerv1.Address{Address: "nope"} }, "E-MAIL-002"},
		"CRLF in subject":     {subject("Hi\r\nBcc: x@evil.example"), "E-MAIL-004"},
		"tab in subject":      {subject("Invoice\t42"), ""},
		"VT in subject":       {subject("Hi\vthere"), "E-MAIL-004"},
		"LS in subject":       {subject("Hi\u2028Bcc: x@evil.example"), "E-MAIL-004"},
		"PS in subject":       {subject("Hi\u2029"), "E-MAIL-004"},
		"invalid UTF-8":       {subject("Hi \x85"), "E-MAIL-004"},
		"CRLF in from name":   {func(m *req) { m.FromName = "Ops\nBcc: x" }, "E-MAIL-004"},
		"DEL in from name":    {func(m *req) { m.FromName = "Ops\x7f" }, "E-MAIL-004"},
		"CRLF in to name":     {func(m *req) { m.To[0].Name = "Ana\r\n" }, "E-MAIL-004"},
		"NEL in to name":      {func(m *req) { m.To[0].Name = "Ana\u0085" }, "E-MAIL-004"},
		"LS in header":        {func(m *req) { m.Headers[0].Value = "<a@acme.example>\u2028Bcc: x@evil.example" }, "E-MAIL-004"},
		"header not allowed":  {func(m *req) { m.Headers = []*runnerv1.Header{{Name: "Bcc", Value: "x@evil.example"}} }, "E-MAIL-004"},
		"non-ASCII header":    {func(m *req) { m.Headers[0].Name = "\u0130n-Reply-To" }, "E-MAIL-004"},
		"RLO in from name":    {func(m *req) { m.FromName = "Ops\u202e" }, "E-MAIL-004"},
		"isolate in to name":  {func(m *req) { m.To[0].Name = "Ana\u2069" }, "E-MAIL-004"},
		"LRE in reply name":   {func(m *req) { m.ReplyTo = &runnerv1.Address{Name: "\u202aAna", Address: "ana@acme.example"} }, "E-MAIL-004"},
		"huge header name":    {func(m *req) { m.Headers[0].Name = strings.Repeat("X", 1<<20) }, "E-MAIL-004"},
		"header twice":        {func(m *req) { m.Headers = append(m.Headers, &runnerv1.Header{Name: "in-reply-to", Value: "<b@x>"}) }, "E-MAIL-004"},
		"attachment path":     {file("../42.pdf"), "E-MAIL-004"},
		"dot file":            {file("."), "E-MAIL-004"},
		"dot-dot file":        {file(".."), "E-MAIL-004"},
		"tab in file name":    {file("42\t.pdf"), "E-MAIL-004"},
		"DEL in file name":    {file("42\x7f.pdf"), "E-MAIL-004"},
		"RLO in file name":    {file("invoice\u202efdp.exe"), "E-MAIL-004"},
		"isolate in name":     {file("42\u2066.pdf"), "E-MAIL-004"},
		"non-ASCII file name": {file("Relatório.pdf"), ""},
		"content type":        {ctype("text/html\r\nX: y"), "E-MAIL-004"},
		"NUL in content type": {ctype("application/pdf; name=\"a\x00b\""), "E-MAIL-004"},
		"long content type":   {ctype("application/pdf; x=" + strings.Repeat("a", 250)), "E-MAIL-004"},
		"too many recipients": {func(m *req) {
			for range MaxRecipients {
				m.Bcc = append(m.Bcc, &runnerv1.Address{Address: "b@acme.example"})
			}
		}, "E-MAIL-005"},
		"too large":           {func(m *req) { m.Attachments[0].Data = make([]byte, MaxSize) }, "E-MAIL-005"},
		"exactly MaxSize":     {fill(0), ""},
		"one byte over":       {fill(1), "E-MAIL-005"},
		"content type counts": {fill(len("application/pdf")), "E-MAIL-005"},
		"no body":             {func(m *req) { m.Text = "" }, "E-MAIL-006"},
	} {
		m := ok()
		c.change(m)
		err := Check(m)
		assert.Equal(t, c.code, errs.Code(err), name)
		if e := (*errs.Error)(nil); errors.As(err, &e) {
			assert.NotRegexp(t, `[\r\n]`, e.Msg, name)
			assert.Less(t, len(e.Msg), 512, name)
		}
	}
}

func TestPage(t *testing.T) {
	for _, p := range [][4]int{{3, 7, 3, 7}, {-1, 0, 0, DefaultList}, {0, -1, 0, DefaultList}, {0, MaxList + 1, 0, MaxList}} {
		offset, limit := Page(p[0], p[1])
		assert.Equal(t, [2]int{p[2], p[3]}, [2]int{offset, limit}, "Page(%d, %d)", p[0], p[1])
	}
}

func TestFolderAndDomain(t *testing.T) {
	require.NoError(t, Folder("archive"))
	assert.Equal(t, "E-MAIL-007", errs.Code(Folder("Spam")))
	assert.Equal(t, "acme.example", Domain("Ana@ACME.example"))
	assert.Empty(t, Domain("a@[127.0.0.1]"))
	assert.Empty(t, Domain("a@x_y.example"))
	assert.True(t, ValidDomain("mail.acme.example"))
	for _, d := range []string{"ACME.example", "*.acme.example", "acme", "a@acme.example", "bücher.example"} {
		assert.False(t, ValidDomain(d), d)
	}
}
