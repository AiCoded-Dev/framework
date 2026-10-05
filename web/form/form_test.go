package form

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func post(t *testing.T, values url.Values) *http.Request {
	t.Helper()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader(values.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	require.NoError(t, r.ParseForm())
	return r
}

func postMultipart(t *testing.T, values, files url.Values) *http.Request {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for name, vs := range values {
		for _, v := range vs {
			require.NoError(t, mw.WriteField(name, v))
		}
	}
	for name, fileNames := range files {
		for _, fileName := range fileNames {
			fw, err := mw.CreateFormFile(name, fileName)
			require.NoError(t, err)
			_, err = fw.Write([]byte("content"))
			require.NoError(t, err)
		}
	}
	require.NoError(t, mw.Close())
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	require.NoError(t, r.ParseMultipartForm(1<<20))
	return r
}

func TestInput(t *testing.T) {
	var in Input[string]
	assert.False(t, in.IsNotNull())
	in.SetValue("hello")
	assert.True(t, in.IsNotNull())
	assert.Equal(t, "hello", in.GetValue())
	in.SetError("bad value")
	assert.True(t, in.HasError())
	assert.Equal(t, "bad value", in.GetError())

	r := post(t, url.Values{"age": {"42"}, "bad": {"x"}, "empty": {""}})
	var age, bad Input[int]
	age.Process(r, "age", false, true)
	assert.False(t, age.HasError())
	assert.Equal(t, 42, age.GetValue())
	bad.Process(r, "bad", false, true)
	assert.Equal(t, "Invalid syntax", bad.GetError())
	assert.Equal(t, "x", bad.GetFormValue())

	var empty, missing Input[string]
	empty.Process(r, "empty", false, true)
	assert.Equal(t, MessageRequiredField, empty.GetError())
	missing.Process(r, "missing", false, false)
	assert.False(t, missing.HasError())
	assert.False(t, missing.IsNotNull())
}

func TestInputMultiple(t *testing.T) {
	var in InputMultiple[int]
	assert.False(t, in.IsNotNull())
	in.SetValue(map[int]struct{}{1: {}})
	assert.True(t, in.IsNotNull())

	var posted, bad, none InputMultiple[int]
	posted.Process(post(t, url.Values{"n": {"1", "2", "2"}}), "n", false, true)
	assert.Equal(t, map[int]struct{}{1: {}, 2: {}}, posted.GetValue())
	bad.Process(post(t, url.Values{"n": {"1", "x"}}), "n", false, false)
	assert.Equal(t, "Invalid syntax", bad.GetError())
	none.Process(post(t, nil), "n", false, true)
	assert.Equal(t, MessageRequiredField, none.GetError())
}

func TestSelect(t *testing.T) {
	opts := []SelectOptionElement[int]{SelectOption[int]{Value: 1}, SelectOption[int]{Value: 2}}

	var s Select[int]
	assert.False(t, s.IsNotNull())
	s.SetValue(42)
	assert.True(t, s.IsNotNull())
	assert.Equal(t, 42, s.GetValue())
	s.SetOptions(opts)
	assert.Equal(t, opts, s.GetOptions())

	r := post(t, url.Values{"bad": {"x"}, "m": {"1", "2"}})
	var missing, optional, bad Select[int]
	for _, f := range []*Select[int]{&missing, &optional, &bad} {
		f.SetOptions(opts)
	}
	missing.Process(r, "s", false, true)
	assert.Equal(t, MessageRequiredField, missing.GetError())
	optional.Process(r, "s", false, false)
	assert.False(t, optional.HasError())
	bad.Process(r, "bad", false, false)
	assert.Equal(t, "Invalid syntax", bad.GetError())

	var m SelectMultiple[int]
	assert.False(t, m.IsNotNull())
	m.SetOptions(opts)
	assert.Equal(t, opts, m.GetOptions())
	m.Process(r, "m", false, true)
	assert.False(t, m.HasError())
	assert.True(t, m.IsNotNull())
	assert.Equal(t, map[int]struct{}{1: {}, 2: {}}, m.GetValue())
}

func TestSelectRefusesValuesNotOffered(t *testing.T) {
	r := post(t, url.Values{"s": {"3"}, "m": {"1", "9"}})
	opts := []SelectOptionElement[int]{
		SelectOption[int]{Value: 1, Label: "one"},
		SelectOptionGroup[int]{Label: "g", Options: []SelectOptionElement[int]{
			SelectOption[int]{Value: 2, Label: "two"},
			SelectOption[int]{Value: 3, Label: "three", Disabled: true},
		}},
	}
	var s Select[int]
	s.SetOptions(opts)
	s.Process(r, "s", false, true)
	assert.Equal(t, MessageInvalidOption, s.GetError())
	assert.Zero(t, s.GetValue())

	var m SelectMultiple[int]
	m.SetOptions(opts)
	m.Process(r, "m", false, false)
	assert.Equal(t, MessageInvalidOption, m.GetError())
	assert.NotContains(t, m.GetValue(), 9)

	var ok Select[int]
	ok.SetOptions(opts)
	ok.Process(post(t, url.Values{"s": {"2"}}), "s", false, true)
	assert.False(t, ok.HasError())
	assert.Equal(t, 2, ok.GetValue())

	var noOptions Select[string]
	noOptions.Process(post(t, url.Values{"s": {"x"}}), "s", false, false)
	assert.Equal(t, MessageInvalidOption, noOptions.GetError())
}

func TestProcessWithoutMultipartForm(t *testing.T) {
	r := post(t, url.Values{"a": {"x"}})
	for _, f := range []interface {
		Process(r *http.Request, name string, isMultipart, isRequired bool)
		GetError() string
	}{&Input[string]{}, &InputMultiple[string]{}, &Select[string]{}, &SelectMultiple[string]{}, &Textarea{}, &File{}, &FileMultiple{}} {
		require.NotPanics(t, func() { f.Process(r, "a", true, true) })
		assert.Equal(t, MessageRequiredField, f.GetError())
	}
}

func TestAllows(t *testing.T) {
	g := SelectOptionGroup[string]{Options: []SelectOptionElement[string]{SelectOption[string]{Value: "a"}}}
	assert.True(t, g.Allows("a"))
	assert.False(t, g.Allows("b"))
	g.Disabled = true
	assert.False(t, g.Allows("a"))
}

func TestTextarea(t *testing.T) {
	var ta Textarea
	assert.False(t, ta.IsNotNull())
	ta.SetValue("some text")
	assert.True(t, ta.IsNotNull())
	assert.Equal(t, "some text", ta.GetValue())

	var posted, empty Textarea
	posted.Process(post(t, url.Values{"t": {"<b>hi</b>"}}), "t", false, true)
	assert.Equal(t, "<b>hi</b>", posted.GetValue())
	empty.Process(post(t, url.Values{"t": {""}}), "t", false, true)
	assert.Equal(t, MessageRequiredField, empty.GetError())
}

func TestFiles(t *testing.T) {
	assert.False(t, IsMultipart(post(t, nil)))
	r := postMultipart(t, url.Values{"title": {"x"}}, url.Values{"one": {"a.txt"}, "many": {"b.txt", "c.txt"}})
	assert.True(t, IsMultipart(r))

	var one, noFile File
	assert.False(t, one.IsNotNull())
	one.Process(r, "one", true, true)
	require.True(t, one.IsNotNull())
	assert.Equal(t, "a.txt", one.GetValue().Filename)
	noFile.Process(r, "none", true, true)
	assert.Equal(t, MessageRequiredField, noFile.GetError())

	var many, noFiles FileMultiple
	assert.False(t, many.IsNotNull())
	many.Process(r, "many", true, true)
	require.Len(t, many.GetValue(), 2)
	assert.Equal(t, "c.txt", many.GetValue()[1].Filename)
	noFiles.Process(r, "none", true, true)
	assert.Equal(t, MessageRequiredField, noFiles.GetError())

	var title Input[string]
	title.Process(r, "title", true, true)
	assert.Equal(t, "x", title.GetValue())
}

func TestBaseFormValues(t *testing.T) {
	var in Input[string]
	var f BaseFormValues
	f.SetElements([]Element{&in})
	assert.False(t, f.HasError())
	assert.False(t, f.IsValidated())
	f.MarkValidated()
	assert.True(t, f.IsValidated())
	in.SetError("required")
	assert.True(t, f.HasError())

	var g BaseFormValues
	g.SetError("form error")
	assert.True(t, g.HasError())
	assert.Equal(t, "form error", g.GetError())
}

func TestPrepare(t *testing.T) {
	var f BaseFormValues
	f.Prepare("abcd1234.add", "tok")
	assert.Equal(t, "abcd1234.add", f.ID())
	assert.Equal(t, "tok", f.Token())
}

func TestParseValue(t *testing.T) {
	var (
		s      string
		i      int
		f      float64
		b      bool
		u      uint8
		errStr string
	)
	parseValue("hello", &s, &errStr)
	parseValue("42", &i, &errStr)
	parseValue("3.14", &f, &errStr)
	parseValue("true", &b, &errStr)
	parseValue("255", &u, &errStr)
	assert.Empty(t, errStr)
	assert.Equal(t, "hello", s)
	assert.Equal(t, 42, i)
	assert.InDelta(t, 3.14, f, 1e-9)
	assert.True(t, b)
	assert.Equal(t, uint8(255), u)

	var notInt, notBool, overflow string
	parseValue("abc", new(int), &notInt)
	parseValue("notabool", new(bool), &notBool)
	parseValue("256", new(uint8), &overflow)
	assert.Equal(t, "Invalid syntax", notInt)
	assert.Equal(t, "Invalid syntax", notBool)
	assert.Equal(t, "Value out of range", overflow)
}

func TestOptionHTML(t *testing.T) {
	selected := func(v int) bool { return v == 1 }
	for want, o := range map[string]SelectOptionElement[int]{
		`<option value="1" selected>One</option>`:   SelectOption[int]{Value: 1, Label: "One"},
		`<option value="2">Two</option>`:            SelectOption[int]{Value: 2, Label: "Two"},
		`<option value="3" disabled>Three</option>`: SelectOption[int]{Value: 3, Label: "Three", Disabled: true},
		`<optgroup label="A&amp;B" disabled><option value="1" selected>One</option></optgroup>`: SelectOptionGroup[int]{
			Label: "A&B", Disabled: true, Options: []SelectOptionElement[int]{SelectOption[int]{Value: 1, Label: "One"}},
		},
	} {
		var b strings.Builder
		require.NoError(t, o.WriteHtml(&b, selected))
		assert.Equal(t, want, b.String())
	}
}

func TestOptionsAreEscaped(t *testing.T) {
	var b strings.Builder
	o := SelectOption[string]{Value: `"><script>`, Label: "<b>x</b>"}
	require.NoError(t, o.WriteHtml(&b, func(string) bool { return false }))
	assert.Equal(t, `<option value="&#34;&gt;&lt;script&gt;">&lt;b&gt;x&lt;/b&gt;</option>`, b.String())
}
