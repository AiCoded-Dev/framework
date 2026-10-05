// Package render writes template values into a page, each with the escaper of the HTML
// context it lands in. Generated code calls it; apps do not.
package render

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"aicoded.dev/framework/web"
)

// UnsafeURL replaces a URL value whose scheme is not http, https or mailto.
const UnsafeURL = "about:invalid#aicoded-unsafe-url"

var escaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&#34;",
	"'", "&#39;",
	"\x00", "�",
)

// Format returns v as display text.
func Format(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case int:
		return strconv.Itoa(x)
	case int8:
		return strconv.FormatInt(int64(x), 10)
	case int16:
		return strconv.FormatInt(int64(x), 10)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case int64:
		return strconv.FormatInt(x, 10)
	case uint:
		return strconv.FormatUint(uint64(x), 10)
	case uint8:
		return strconv.FormatUint(uint64(x), 10)
	case uint16:
		return strconv.FormatUint(uint64(x), 10)
	case uint32:
		return strconv.FormatUint(uint64(x), 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	case float32:
		return strconv.FormatFloat(float64(x), 'g', -1, 32)
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	default:
		return fmt.Sprint(v)
	}
}

func escape(w io.Writer, s string) error {
	_, err := escaper.WriteString(w, s)
	return err
}

// Text writes v as element text, including the text of <title> and <textarea>.
func Text(w io.Writer, v any) error { return escape(w, Format(v)) }

// Attr writes v inside a double-quoted attribute value.
func Attr(w io.Writer, v any) error { return escape(w, Format(v)) }

// URL writes v as a whole URL attribute value, as in href="{{ link }}". A value whose scheme
// is not http, https or mailto becomes UnsafeURL.
func URL(w io.Writer, v any) error {
	s := Format(v)
	if !allowedScheme(s) {
		s = UnsafeURL
	}
	return escape(w, processURL(s, true))
}

// URLPart writes v in a URL attribute value after a fixed path or scheme and before any ? or
// #, as in href="/notes/{{ id }}".
func URLPart(w io.Writer, v any) error { return escape(w, processURL(Format(v), true)) }

// URLPathStart writes v in a URL attribute value right after its leading "/", as in
// href="/{{ slug }}". It encodes a leading "/" of v and writes an empty v as ".", so the URL
// never starts with "//" or "/\", which browsers read as the address of another site.
func URLPathStart(w io.Writer, v any) error {
	s := processURL(Format(v), true)
	switch {
	case s == "":
		s = "."
	case s[0] == '/':
		s = "%2f" + s[1:]
	}
	return escape(w, s)
}

// URLQuery writes v in a URL attribute value after ? or #, as in href="/find?q={{ q }}".
func URLQuery(w io.Writer, v any) error { return escape(w, processURL(Format(v), false)) }

// JSON writes v as the body of a <script type="application/json"> element.
func JSON(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

// HTML writes markup that is already safe.
func HTML(w io.Writer, v web.SafeHTML) error {
	_, err := io.WriteString(w, v.String())
	return err
}

// If returns whenTrue if cond holds and whenFalse otherwise. Templates write it as cond ? a : b.
func If[T any](cond bool, whenTrue, whenFalse T) T {
	if cond {
		return whenTrue
	}
	return whenFalse
}

// allowedScheme reports whether s is a relative URL or has the scheme http, https or mailto.
func allowedScheme(s string) bool {
	i := strings.IndexAny(s, ":/?#")
	if i < 0 || s[i] != ':' {
		return true
	}
	switch strings.ToLower(s[:i]) {
	case "http", "https", "mailto":
		return true
	}
	return false
}

// processURL percent-encodes the bytes of s that may not appear in a URL. With norm, the
// reserved characters and % are kept, so a URL keeps its structure; without it, they are
// encoded, so a value stays one query or fragment component.
func processURL(s string, norm bool) string {
	var b strings.Builder
	written := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '!', '#', '$', '&', '*', '+', ',', '/', ':', ';', '=', '?', '@', '[', ']', '%':
			if norm {
				continue
			}
		case '-', '.', '_', '~':
			continue
		default:
			if 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' {
				continue
			}
		}
		b.WriteString(s[written:i])
		fmt.Fprintf(&b, "%%%02x", c)
		written = i + 1
	}
	b.WriteString(s[written:])
	return b.String()
}
