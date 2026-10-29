package migrate

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"aicoded.dev/framework/internal/errs"
)

const (
	// draftFile is where an app keeps its draft; problems name their line in it.
	draftFile = ".aicoded/migration.sql"
	// maxDraft is the most bytes a draft may hold.
	maxDraft = 64 << 10
	// maxStatements is the most statements a draft may hold.
	maxStatements = 100
)

// Split splits a draft into its statements at each ; outside strings, quoted names and comments.
// It returns them without comments and surrounding space, and refuses, with E-MIG-001, a draft
// over 64 KiB, not UTF-8, with no statement or with more than 100.
func Split(draft []byte) ([]string, error) {
	stmts, err := split(draft)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(stmts))
	for i, s := range stmts {
		out[i] = s.text
	}
	return out, nil
}

// Normalize returns the form of stmt, a statement as Split returns it, in which two statements that
// MySQL reads alike are the same: its tokens with one space between each two, without comments or
// a final ;, and each SQL keyword, type and function of a fixed list in upper case. Every other
// word, a name, keeps its case, since MySQL tells tables apart by the case of their names.
// Strings and names in backquotes are kept as they are. Text the checks cannot read is returned
// without surrounding space.
func Normalize(stmt string) string {
	toks, _, err := lex(stmt)
	if err != nil {
		return strings.TrimSpace(stmt)
	}
	toks = slices.DeleteFunc(toks, func(t token) bool { return t.kind == comment })
	if n := len(toks); n > 0 && toks[n-1].is(";") {
		toks = toks[:n-1]
	}
	words := make([]string, len(toks))
	for i, t := range toks {
		words[i] = t.text
		if up := t.up(); keywords[up] {
			words[i] = up
		}
	}
	return strings.Join(words, " ")
}

// statement is a statement of a draft and the line it starts on.
type statement struct {
	text string
	line int
}

func split(draft []byte) ([]statement, error) {
	if len(draft) > maxDraft {
		return nil, errs.New("E-MIG-001", fmt.Sprintf("the draft holds %d bytes, more than the %d it may", len(draft), maxDraft),
			"split the change: publish part of it, then write the rest in a new draft")
	}
	if !utf8.Valid(draft) {
		return nil, errs.At(position(invalidLine(draft)), "E-MIG-001", "the draft is not UTF-8 text", "save .aicoded/migration.sql as UTF-8")
	}
	src := string(draft)
	toks, line, err := lex(src)
	if err != nil {
		return nil, at(line, err)
	}
	var (
		out   []statement
		b     strings.Builder
		last  int
		first int
	)
	flush := func(end int) {
		b.WriteString(src[last:end])
		if s := strings.TrimSpace(b.String()); s != "" {
			out = append(out, statement{s, first})
		}
		b.Reset()
		first = 0
	}
	for _, t := range toks {
		switch {
		case t.kind == comment:
			b.WriteString(src[last:t.start])
			b.WriteByte(' ')
			last = t.end
		case t.is(";"):
			flush(t.start)
			last = t.end
		case first == 0:
			first = t.line
		}
	}
	flush(len(src))
	switch {
	case len(out) == 0:
		return nil, errs.New("E-MIG-001", "the draft holds no statement", "write the schema change in .aicoded/migration.sql, or delete the file")
	case len(out) > maxStatements:
		return nil, errs.New("E-MIG-001", fmt.Sprintf("the draft holds %d statements, more than the %d it may", len(out), maxStatements),
			"split the change: publish part of it, then write the rest in a new draft")
	}
	return out, nil
}

// position names line of the draft as file:line.
func position(line int) string {
	return draftFile + ":" + strconv.Itoa(line)
}

// at returns err, a coded error, at line of the draft.
func at(line int, err *errs.Error) *errs.Error {
	return errs.At(position(line), err.Code, err.Msg, err.Fix)
}

// invalidLine returns the line of the first byte of draft that is not UTF-8.
func invalidLine(draft []byte) int {
	line := 1
	for len(draft) > 0 {
		r, n := utf8.DecodeRune(draft)
		if r == utf8.RuneError && n <= 1 {
			break
		}
		if r == '\n' {
			line++
		}
		draft = draft[n:]
	}
	return line
}

// kind is the kind of a token.
type kind int

const (
	word    kind = iota // a keyword or a plain name
	ident               // a name in backquotes
	str                 // a string in single or double quotes
	number              // a number
	punct               // one other character
	hidden              // an executable comment /*! … */ or an optimizer hint /*+ … */
	comment             // a comment, which MySQL skips
)

// token is a token of SQL text, at src[start:end], starting on line.
type token struct {
	kind       kind
	text       string
	start, end int
	line       int
}

// is reports whether t is the punctuation p.
func (t token) is(p string) bool {
	return t.kind == punct && t.text == p
}

// up returns t's text in upper case when t is a word, and "" otherwise.
func (t token) up() string {
	if t.kind != word {
		return ""
	}
	return strings.ToUpper(t.text)
}

// isName reports whether t can be a name: a word or a name in backquotes.
func (t token) isName() bool {
	return t.kind == word || t.kind == ident
}

// name returns the name t stands for.
func (t token) name() string {
	if t.kind == ident {
		return strings.ReplaceAll(t.text[1:len(t.text)-1], "``", "`")
	}
	return t.text
}

// lex splits src into tokens as MySQL reads them. On text it cannot read, it returns the line
// and an E-MIG-002 error.
func lex(src string) ([]token, int, *errs.Error) {
	var toks []token
	line := 1
	for i := 0; i < len(src); {
		c := src[i]
		k, end := punct, i+1
		switch {
		case c == '\n':
			line++
			i++
			continue
		case c == ' ' || c == '\t' || c == '\r' || c == '\f' || c == '\v':
			i++
			continue
		case c == '#' || strings.HasPrefix(src[i:], "--") && (i+2 == len(src) || src[i+2] <= ' '):
			k, end = comment, len(src)
			if n := strings.IndexByte(src[i:], '\n'); n >= 0 {
				end = i + n
			}
		case strings.HasPrefix(src[i:], "/*"):
			n := strings.Index(src[i+2:], "*/")
			if n < 0 {
				return nil, line, unreadable("a comment that does not end")
			}
			k, end = comment, i+2+n+2
			if i+2 < len(src) && (src[i+2] == '!' || src[i+2] == '+') {
				k = hidden
			}
		case c == '\'' || c == '"' || c == '`':
			var ok bool
			if end, ok = quoted(src, i); !ok {
				return nil, line, unreadable("a string or a name in quotes that does not end")
			}
			k = str
			if c == '`' {
				k = ident
			}
		case isDigit(c) || c == '.' && i+1 < len(src) && isDigit(src[i+1]) && !afterName(toks):
			k, end = number, numberEnd(src, i)
			if end < len(src) && isIdent(src[end]) {
				if strings.Contains(src[i:end], ".") {
					return nil, line, unreadable("a number followed by letters")
				}
				k, end = word, identEnd(src, i)
			}
		case isIdent(c):
			k, end = word, identEnd(src, i)
		}
		toks = append(toks, token{k, src[i:end], i, end, line})
		line += strings.Count(src[i:end], "\n")
		i = end
	}
	return toks, line, nil
}

// unreadable returns the E-MIG-002 error for text the checks cannot read.
func unreadable(what string) *errs.Error {
	return errs.New("E-MIG-002", "the checks cannot read "+what, "close every string, name in backquotes and comment")
}

// quoted returns the end of the string or name in quotes that starts at src[i].
func quoted(src string, i int) (int, bool) {
	q := src[i]
	for j := i + 1; j < len(src); j++ {
		switch {
		case src[j] == q && j+1 < len(src) && src[j+1] == q:
			j++
		case src[j] == q:
			return j + 1, true
		case src[j] == '\\' && q != '`':
			j++
		}
	}
	return 0, false
}

// numberEnd returns the end of the number that starts at src[i]: digits, an optional fraction
// and an optional exponent.
func numberEnd(src string, i int) int {
	digits := func(j int) int {
		for j < len(src) && isDigit(src[j]) {
			j++
		}
		return j
	}
	j := digits(i)
	if j < len(src) && src[j] == '.' {
		j = digits(j + 1)
	}
	if j < len(src) && (src[j] == 'e' || src[j] == 'E') {
		k := j + 1
		if k < len(src) && (src[k] == '+' || src[k] == '-') {
			k++
		}
		if k < len(src) && isDigit(src[k]) {
			j = digits(k)
		}
	}
	return j
}

// identEnd returns the end of the word that starts at src[i].
func identEnd(src string, i int) int {
	for i < len(src) && isIdent(src[i]) {
		i++
	}
	return i
}

// afterName reports whether the last token that is not a comment is a name.
func afterName(toks []token) bool {
	for i := len(toks) - 1; i >= 0; i-- {
		if toks[i].kind != comment {
			return toks[i].isName()
		}
	}
	return false
}

func isDigit(c byte) bool {
	return '0' <= c && c <= '9'
}

// isIdent reports whether c can be part of a name not in quotes, as MySQL allows: ASCII
// letters, digits, $, _ and any byte of a character beyond ASCII.
func isIdent(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || isDigit(c) || c == '$' || c == '_' || c >= utf8.RuneSelf
}
