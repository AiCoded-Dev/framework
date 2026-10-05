package template

import (
	"regexp"
	"strconv"
	"strings"

	"aicoded.dev/framework/cmd/aicoded/internal/generate/template/node"
	"aicoded.dev/framework/internal/errs"
)

func init() { yyErrorVerbose = true }

type exprLex struct {
	file       string // base name, for node positions
	display    string // path in error positions
	text       string
	insideExpr bool
	curLine    int
	err        *errs.Error
	result     *node.Content

	// exprSources holds the source of every {{ }} and {{$ }} in lexing order.
	exprSources    []string
	exprTextAtOpen string

	// last is the token Lex returned last.
	last int
}

var simpleTokens = []struct {
	token string
	value int
}{
	{"==", EQ},
	{"!=", NE},
	{">=", GE},
	{"<=", LE},
	{"&&", AND},
	{"||", OR},
	{"}}", EXPR_END},
	{"!", NOT},
}

var reTokens = []struct {
	re    *regexp.Regexp
	value int
}{
	{regexp.MustCompile(`^(?i)IN(?:\s|$)`), IN},
	{regexp.MustCompile(`^"[^\\"]*(?:\\.[^\\"]*)*"`), STRING},
	{regexp.MustCompile("^`[^`]*`"), STRING},
	{regexp.MustCompile(`^'[^\\']*(?:\\.[^\\']*)*'`), STRING},
	{regexp.MustCompile(`^-?\d+(?:\.\d+)?`), NUMBER},
	{regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*`), IDENTIFIER},
}

// Error records the first error of a parse.
func (x *exprLex) Error(s string) {
	if x.err == nil {
		x.err = errAt(x.display, x.curLine, "E-GEN-015", fixes["E-GEN-015"], "cannot parse the expression: %s", readable(s))
	}
}

var tokenNames = map[string]string{
	"$end": "end of text", "$unk": "character", "TEXT": "text",
	"EXPR_START": "{{", "RAW_EXPR_START": "{{$", "EXPR_END": "}}",
	"IDENTIFIER": "name", "STRING": "string", "NUMBER": "number", "IN": "in",
	"EQ": "==", "NE": "!=", "GE": ">=", "LE": "<=", "OR": "||", "AND": "&&", "NOT": "!",
}

// readable replaces the parser's token names in msg with what the template shows.
func readable(msg string) string {
	words := strings.Split(strings.TrimPrefix(msg, "syntax error: "), " ")
	for i, w := range words {
		tok := strings.TrimSuffix(w, ",")
		if name, ok := tokenNames[tok]; ok {
			words[i] = name + w[len(tok):]
		}
	}
	return strings.Join(words, " ")
}

func (x *exprLex) Lex(yylval *yySymType) int {
	x.last = x.lex(yylval)
	return x.last
}

// afterOperand reports whether the last token ends an operand, so a - that follows it is a
// subtraction and not the sign of a number.
func (x *exprLex) afterOperand() bool {
	switch x.last {
	case IDENTIFIER, NUMBER, STRING, ')', ']':
		return true
	}
	return false
}

func (x *exprLex) lex(yylval *yySymType) int {
	for {
		if len(x.text) == 0 {
			return 0
		}

		if !x.insideExpr {
			return x.lexText(yylval)
		}

		for _, token := range simpleTokens {
			if strings.HasPrefix(x.text, token.token) {
				if token.value == EXPR_END {
					consumed := len(x.exprTextAtOpen) - len(x.text)
					x.exprSources = append(x.exprSources, strings.TrimSpace(x.exprTextAtOpen[:consumed]))
					x.exprTextAtOpen = ""
					x.insideExpr = false
				}
				x.text = x.text[len(token.token):]
				yylval.string = token.token
				return token.value
			}
		}

		for _, token := range reTokens {
			if token.value == NUMBER && x.text[0] == '-' && x.afterOperand() {
				continue
			}
			if m := token.re.FindString(x.text); m != "" {
				x.text = x.text[len(m):]
				yylval.string = m
				if token.value == STRING {
					s, err := unquote(m)
					if err != nil {
						x.Error("invalid string " + m)
						return 0
					}
					yylval.string = strconv.Quote(s)
				}
				return token.value
			}
		}

		c := x.text[0]
		x.text = x.text[1:]
		switch c {
		case ' ', '\t', '\r':
			continue
		case '\n':
			x.curLine++
			continue
		default:
			return int(c)
		}
	}
}

func (x *exprLex) lexText(yylval *yySymType) int {
	if strings.HasPrefix(x.text, "{{") {
		open := "{{"
		tok := EXPR_START
		if strings.HasPrefix(x.text, "{{$") {
			open, tok = "{{$", RAW_EXPR_START
		}
		yylval.string = open
		x.text = x.text[len(open):]
		x.insideExpr = true
		x.exprTextAtOpen = x.text
		return tok
	}
	end := strings.Index(x.text, "{{")
	if end < 0 {
		end = len(x.text)
	}
	yylval.string = x.text[:end]
	x.text = x.text[end:]
	x.curLine += strings.Count(yylval.string, "\n")
	return TEXT
}

// unquote returns the value of a string literal. Double-quoted and single-quoted strings take
// Go escapes; back-quoted strings are raw.
func unquote(lit string) (string, error) {
	switch lit[0] {
	case '`':
		return lit[1 : len(lit)-1], nil
	case '\'':
		body := lit[1 : len(lit)-1]
		var b strings.Builder
		for i := 0; i < len(body); i++ {
			switch {
			case body[i] == '\\' && i+1 < len(body) && body[i+1] == '\'':
				b.WriteByte('\'')
				i++
			case body[i] == '\\' && i+1 < len(body):
				b.WriteString(body[i : i+2])
				i++
			case body[i] == '"':
				b.WriteString(`\"`)
			default:
				b.WriteByte(body[i])
			}
		}
		return strconv.Unquote(`"` + b.String() + `"`)
	}
	return strconv.Unquote(lit)
}

func bn(lexer yyLexer) node.BaseNode {
	x := lexer.(*exprLex)
	return node.BaseNode{File: x.file, Line: x.curLine}
}
