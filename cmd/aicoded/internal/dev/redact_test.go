package dev

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
	"aicoded.dev/framework/cmd/aicoded/internal/manifest"
	"aicoded.dev/framework/cmd/aicoded/internal/problem"
	"aicoded.dev/framework/cmd/aicoded/internal/workspace"
	"aicoded.dev/framework/runnerproto/runnerv1"
)

const leaked = "s3cr3t-value"

// secretStore returns the store of the app hello, whose secrets have the values secrets, and
// what it prints.
func secretStore(secrets map[string]string) (*store, *syncBuffer) {
	var out syncBuffer
	s := newStore("hello", &out)
	s.hideSecrets(secrets)
	return s, &out
}

// jsonOf returns v as JSON.
func jsonOf(t *testing.T, v any) string {
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return string(data)
}

func TestRedactLogLines(t *testing.T) {
	s, out := secretStore(map[string]string{"token": leaked, "quoted": `pa"ss<word>`})
	s.write(sourceApp, "plain "+leaked)
	s.write(sourceApp, `{"time":"2026-09-29T10:00:00Z","level":"INFO","msg":"got `+leaked+`","req":{"headers":["Bearer `+leaked+`"]}}`)
	s.write(sourceApp, `{"time":"2026-09-29T10:00:00Z","level":"INFO","msg":"pa\"ss<word>","raw":"pa\"ss<word>"}`)

	entries := s.logs.All()
	require.Len(t, entries, 3)
	assert.Equal(t, "plain [secret token]", entries[0].Message)
	assert.Equal(t, "got [secret token]", entries[1].Message)
	assert.Equal(t, map[string]any{"headers": []any{"Bearer [secret token]"}}, entries[1].Attrs["req"])
	assert.Equal(t, "[secret quoted]", entries[2].Message, "the value as it looks in JSON")
	assert.Equal(t, "[secret quoted]", entries[2].Attrs["raw"], "with HTML escaping too")
	assert.NotContains(t, jsonOf(t, entries), leaked)
	assert.Contains(t, out.String(), "hello | plain [secret token]\n")
	assert.NotContains(t, out.String(), leaked)
	assert.NotContains(t, out.String(), `pa\"ss`)
}

func TestRedactSpans(t *testing.T) {
	s, _ := secretStore(map[string]string{"token": leaked})
	s.addSpans([]*runnerv1.Span{{Name: "call " + leaked, Attributes: map[string]string{"auth": "Bearer " + leaked, leaked: "key"}, Error: "refused " + leaked}})
	spans := s.spans.All()
	require.Len(t, spans, 1)
	assert.Equal(t, "call [secret token]", spans[0].Name)
	assert.Equal(t, map[string]string{"auth": "Bearer [secret token]", "[secret token]": "key"}, spans[0].Attrs)
	assert.Equal(t, "refused [secret token]", spans[0].Error)
}

func TestRedactMail(t *testing.T) {
	s, _ := secretStore(map[string]string{"token": leaked})
	s.mail.setEmail(acme)
	m := msg("ana@acme.example")
	m.Subject, m.Text, m.Html = "key "+leaked, "the key is "+leaked, "<b>"+leaked+"</b>"
	m.Attachments = []*runnerv1.Attachment{{Filename: leaked + ".txt", ContentType: "text/plain", Data: []byte(leaked)}}
	_, err := s.mail.Send(t.Context(), connect.NewRequest(m))
	require.NoError(t, err)
	s.mail.deliver(&runnerv1.GetResponse{Summary: &runnerv1.MessageSummary{Subject: "re: " + leaked}, Text: leaked})

	sent, ok := s.mail.get("sent-1")
	require.True(t, ok)
	assert.Equal(t, "key [secret token]", sent.Subject)
	assert.Equal(t, "the key is [secret token]", sent.Text)
	assert.Equal(t, "<b>[secret token]</b>", sent.HTML)
	assert.NotContains(t, jsonOf(t, s.mail.sentMail()), leaked, "the kept message, attachments included")
	assert.NotContains(t, jsonOf(t, s.mail.summaries()), leaked)
}

func TestRedactShortSecret(t *testing.T) {
	secrets := map[string]string{"pin": "123", "token": leaked}
	m := manifest.Manifest{App: "hello", Secrets: []string{"pin", "token"}}
	assert.Equal(t, []string{"pin"}, shortSecrets(m, devconfig.AppValues{Secrets: secrets}))
	s, _ := secretStore(secrets)
	s.write(sourceApp, "pin 123 token "+leaked)
	assert.Equal(t, "pin 123 token [secret token]", s.logs.All()[0].Message, "a value under 4 bytes is not hidden")
}

func TestRedactLongerFirst(t *testing.T) {
	r := newRedactor(map[string]string{"s3cr3t": "short", "s3cr3t-value": "long"})
	assert.Equal(t, "a [secret long] b", r.String("a s3cr3t-value b"))
	assert.Equal(t, "[secret short]", r.String("s3cr3t"))
	assert.Equal(t, "x", (*redactor)(nil).String("x"))
}

func TestRedactOverlaps(t *testing.T) {
	r := newRedactor(map[string]string{"abcdefgh": "one", "efghijkl": "two"})
	assert.Equal(t, "[secret one]", r.String("abcdefghijkl"), "the union of both, under the mark that starts first")
	assert.Equal(t, "x [secret one] [secret one] y", r.String("x abcdefgh abcdefgh y"))
	assert.Equal(t, "[secret one]", r.String("abcdefghabcdefgh"), "adjacent occurrences")

	r = newRedactor(map[string]string{"cr3t": "inner", leaked: "outer"})
	assert.Equal(t, "a [secret outer] b [secret inner]", r.String("a "+leaked+" b cr3t"), "the longer one wins")
}

func TestRedactMultiLineSecret(t *testing.T) {
	const first, second = "-----BEGIN KEY----- MIIBOgIBAAJBAKj34GkxFhD9", "qZ8lN3nOv2r4Wc8yS1tDEg -----END KEY-----"
	s, out := secretStore(map[string]string{"key": first + "\r\n" + second})
	fmt.Fprintln(s.writer(sourceApp), first+"\r\n"+second)

	entries := s.logs.All()
	require.Len(t, entries, 2, "one entry per printed line")
	assert.Equal(t, "[secret key]\r", entries[0].Message)
	assert.Equal(t, "[secret key]", entries[1].Message)
	for _, line := range []string{first, second} {
		assert.NotContains(t, jsonOf(t, entries), line)
		assert.NotContains(t, out.String(), line)
	}
}

func TestRedactEarlierValues(t *testing.T) {
	s, _ := secretStore(map[string]string{"token": leaked})
	s.hideSecrets(map[string]string{"token": "n3w-value"})
	s.write(sourceApp, leaked+" n3w-value")
	assert.Equal(t, "[secret token] [secret token]", s.logs.All()[0].Message, "a value of an earlier start stays hidden")
}

func TestRedactProblems(t *testing.T) {
	var out syncBuffer
	w := testWorkspace("hello")
	w.out = &out
	signer, err := NewSigner()
	require.NoError(t, err)
	w.gateway = NewGateway(8080, nil, signer, nil)
	s := w.slots[0]
	s.store.hideSecrets(map[string]string{"token": leaked})
	w.fail(s, sourceBuild, []problem.Problem{{App: "hello", Code: "E-CHK-002", Pos: "main.go:3", Message: `cannot use "` + leaked + `" as int`, Fix: "fix it"}})

	assert.Equal(t, `cannot use "[secret token]" as int`, w.appStatus(s).Problems[0].Message)
	assert.NotContains(t, out.String(), leaked)
	assert.NotContains(t, jsonOf(t, s.store.logs.All()), leaked)
	page := serve(w.gateway, http.MethodGet, "hello.localhost:8080", "/", nil, "")
	assert.Contains(t, page.Body.String(), "[secret token]")
	assert.NotContains(t, page.Body.String(), leaked)
}

func TestWorkspaceHidesTheMySQLDSN(t *testing.T) {
	root := copyHello(t, t.TempDir(), "hello")
	var lc net.ListenConfig
	l, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	const dsn = "root:s3cret-pw@tcp(127.0.0.1:3306)/"
	cfg := Config{Root: root, State: t.TempDir(), Dev: devconfig.Workspace{MySQL: dsn}, Out: io.Discard}
	w, err := newWorkspace(t.Context(), cfg, []workspace.App{{Name: "hello", Dir: root}}, l, strings.Repeat("a", 64))
	require.NoError(t, err)
	defer w.cancel()
	st := w.slots[0].store
	st.write(sourceRunner, "database: cannot reach "+dsn+" as s3cret-pw")
	assert.Equal(t, "database: cannot reach [secret mysql] as [secret mysql password]", st.logs.All()[0].Message)
}

func TestRedactorCut(t *testing.T) {
	r := newRedactor(map[string]string{leaked: "token"})
	for s, want := range map[string]int{
		"plain":                  5,
		"a " + leaked:            len("a " + leaked),
		"a " + leaked[:5]:        2,
		"a " + leaked[:11] + "x": len("a " + leaked[:11] + "x"),
	} {
		assert.Equal(t, want, r.cut(s), "%q", s)
	}
	overlaps := newRedactor(map[string]string{"abcdefgh": "one", "efghijkl": "two"})
	assert.Equal(t, 2, overlaps.cut("xxabcdefghij"), "the whole secret that the start of another overlaps stays too")
	assert.Equal(t, 5, (*redactor)(nil).cut("plain"))
}

func TestRedactAcrossALongLine(t *testing.T) {
	s, out := secretStore(map[string]string{"token": leaked})
	w := s.writer(sourceApp)
	fmt.Fprint(w, strings.Repeat("x", maxLine)+leaked[:5])
	fmt.Fprint(w, leaked[5:]+" "+strings.Repeat("y", maxLine)+leaked)
	fmt.Fprintln(w, " end")

	var got strings.Builder
	for _, e := range s.logs.All() {
		got.WriteString(e.Message)
	}
	assert.Equal(t, strings.Repeat("x", maxLine)+"[secret token] "+strings.Repeat("y", maxLine)+"[secret token] end", got.String())
	for _, part := range []string{leaked[:5], leaked[5:]} {
		assert.NotContains(t, jsonOf(t, s.logs.All()), part)
		assert.NotContains(t, out.String(), part)
	}
}
