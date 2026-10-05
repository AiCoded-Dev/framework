package lint

import (
	"cmp"
	"go/ast"
	"go/constant"
	"go/types"
	"maps"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// sqlMethods are the methods of database/sql that take SQL, as refused names them. The SQL is
// their first argument after the context, when they take one.
var sqlMethods = func() []string {
	var out []string
	for _, typ := range []string{"DB", "Tx", "Conn"} {
		for _, m := range []string{"Exec", "Prepare", "Query", "QueryRow"} {
			out = append(out, "database/sql."+typ+"."+m, "database/sql."+typ+"."+m+"Context")
		}
	}
	return out
}()

// constantSQL reports, with E-LINT-007, SQL that is not a constant expression, that holds an
// executable comment, a statement of a kind allowedStatement refuses, more than one statement or
// a file access of the database server, a method of sqlMethods that is not called directly,
// which would hide its SQL, and a method that lint cannot tell from one of sqlMethods. It reads generated files too,
// which hold the code of the templates. It records the write of every constant statement.
func (a *app) constantSQL(p *analysis.Pass) {
	for _, f := range p.Files {
		called := map[*ast.Ident]bool{}
		ast.Inspect(f, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				a.sqlCall(p, call, called)
			}
			return true
		})
		ast.Inspect(f, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if !ok || called[id] {
				return true
			}
			obj := p.TypesInfo.Uses[id]
			if m := a.sqlMethod(obj); m != "" {
				diagnose(p, id.Pos(), "E-LINT-007", display(m)+" is used as a value: call it directly, with constant SQL")
			} else if a.unmatchedSQL(obj) {
				diagnose(p, id.Pos(), "E-LINT-007", id.Name+" has type parameters in its signature, so lint cannot tell whether it takes SQL: give the method another name")
			}
			return true
		})
	}
}

// sqlCall checks call when it calls a method of sqlMethods, and adds its method to called.
func (a *app) sqlCall(p *analysis.Pass, call *ast.CallExpr, called map[*ast.Ident]bool) {
	sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return
	}
	m := a.sqlMethod(p.TypesInfo.Uses[sel.Sel])
	if m == "" {
		return
	}
	called[sel.Sel] = true
	i := 0
	if strings.HasSuffix(m, "Context") {
		i++
	}
	if s := p.TypesInfo.Selections[sel]; s != nil && s.Kind() == types.MethodExpr {
		i++
	}
	if i >= len(call.Args) || isTuple(p.TypesInfo.TypeOf(call.Args[i])) {
		diagnose(p, call.Args[0].Pos(), "E-LINT-007", "the SQL passed to "+display(m)+" comes from a call with several results, so it is not a constant")
		return
	}
	tv := p.TypesInfo.Types[call.Args[i]]
	if tv.Value == nil || tv.Value.Kind() != constant.String {
		diagnose(p, call.Args[i].Pos(), "E-LINT-007", "the SQL passed to "+display(m)+" is not a constant")
		return
	}
	sql := constant.StringVal(tv.Value)
	if executable(sql) {
		diagnose(p, call.Args[i].Pos(), "E-LINT-007", "the SQL passed to "+display(m)+" holds an executable comment, which runs as SQL that lint does not read")
		return
	}
	if !allowedStatement(sql) {
		diagnose(p, call.Args[i].Pos(), "E-LINT-007", "the SQL passed to "+display(m)+" is not one of the statement kinds lint allows: "+allowedKinds)
		return
	}
	if words := sqlWords(sql); stacked(words) {
		diagnose(p, call.Args[i].Pos(), "E-LINT-007", "the SQL passed to "+display(m)+" holds more than one statement: pass each statement in a call of its own")
		return
	} else if fileAccess(words) {
		diagnose(p, call.Args[i].Pos(), "E-LINT-007", "the SQL passed to "+display(m)+" reads or writes a file of the database server with INTO OUTFILE, INTO DUMPFILE or LOAD_FILE")
		return
	}
	if w, ok := writeOf(sql); ok {
		a.mu.Lock()
		a.writes[w] = true
		a.mu.Unlock()
	}
}

// sqlMethod returns the method of sqlMethods that obj is, or that obj, a method of an interface,
// stands for. It returns "" for any other obj.
func (a *app) sqlMethod(obj types.Object) string {
	if obj == nil || obj.Pkg() == nil {
		return ""
	}
	if name := member(obj); slices.Contains(sqlMethods, name) {
		return name
	}
	if m := standsFor(obj, a.sqlMethods); m != nil {
		return member(m)
	}
	return ""
}

// unmatchedSQL reports whether obj is a method of an interface, or of a type parameter, with the
// name of a method of sqlMethods and a type parameter in its signature, which stands for none of
// them.
func (a *app) unmatchedSQL(obj types.Object) bool {
	fn := interfaceMethod(obj)
	return fn != nil && len(a.sqlMethods[fn.Name()]) > 0 && hasTypeParam(fn.Type()) && standsFor(fn, a.sqlMethods) == nil
}

// isTuple reports whether t is the type of a call with several results.
func isTuple(t types.Type) bool {
	_, ok := t.(*types.Tuple)
	return ok
}

// executable reports whether sql holds a comment that MySQL or MariaDB runs as SQL: /*! or /*M!.
func executable(sql string) bool {
	return strings.Contains(sql, "/*!") || strings.Contains(strings.ToUpper(sql), "/*M!")
}

// allowedKinds names the statement kinds that allowedStatement accepts, for the finding.
const allowedKinds = "SELECT, INSERT, UPDATE, DELETE, REPLACE, WITH, CREATE TABLE, CREATE INDEX, ALTER TABLE, DROP TABLE and DROP INDEX"

// allowedStatement reports whether sql is one of the statement kinds E-LINT-007 allows. Any other
// kind is refused, so that a statement that runs viewer data as SQL (PREPARE/EXECUTE), calls a
// procedure (CALL) or changes how SQL is lexed (SET) cannot pass as constant SQL.
func allowedStatement(sql string) bool {
	kw := leadingKeywords(sqlWords(sql))
	if len(kw) == 0 {
		return false
	}
	switch kw[0] {
	case "SELECT", "INSERT", "UPDATE", "DELETE", "REPLACE", "WITH":
		return true
	case "CREATE":
		return kwAt(kw, 1) == "TABLE" || kwAt(kw, 1) == "INDEX" || kwAt(kw, 1) == "UNIQUE" && kwAt(kw, 2) == "INDEX"
	case "ALTER":
		return kwAt(kw, 1) == "TABLE"
	case "DROP":
		return kwAt(kw, 1) == "TABLE" || kwAt(kw, 1) == "INDEX"
	}
	return false
}

// stacked reports whether words, the words of a statement, hold a ; that another word follows,
// other than more ; at the end.
func stacked(words []string) bool {
	end := len(words)
	for end > 0 && words[end-1] == ";" {
		end--
	}
	return slices.Contains(words[:end], ";")
}

// fileAccess reports whether words, the words of a statement, read or write a file of the
// database server: INTO OUTFILE, INTO DUMPFILE or LOAD_FILE.
func fileAccess(words []string) bool {
	for i, w := range words {
		next := ""
		if i+1 < len(words) {
			next = strings.ToUpper(words[i+1])
		}
		if strings.EqualFold(w, "LOAD_FILE") || strings.EqualFold(w, "INTO") && (next == "OUTFILE" || next == "DUMPFILE") {
			return true
		}
	}
	return false
}

// leadingKeywords returns up to three leading name words of words, uppercased, skipping the
// parentheses that may wrap a query, and stopping at the first word that is not a name.
func leadingKeywords(words []string) []string {
	var kw []string
	for _, w := range words {
		if w == "(" {
			continue
		}
		if !isName(w) {
			break
		}
		kw = append(kw, strings.ToUpper(w))
		if len(kw) == 3 {
			break
		}
	}
	return kw
}

// kwAt returns the keyword at index i of kw, or "".
func kwAt(kw []string, i int) string {
	if i < len(kw) {
		return kw[i]
	}
	return ""
}

// sortedWrites returns the recorded writes, sorted by table and operation.
func (a *app) sortedWrites() []Write {
	return slices.SortedFunc(maps.Keys(a.writes), func(x, y Write) int {
		return cmp.Or(cmp.Compare(x.Table, y.Table), cmp.Compare(x.Op, y.Op))
	})
}

// writeOps are the statements that write a table.
var writeOps = []string{"DELETE", "INSERT", "REPLACE", "UPDATE"}

// writeOf returns the table that the SQL statement sql writes, and how, when its first words, or
// the first words after its WITH clause, make it an INSERT, UPDATE, DELETE or REPLACE.
func writeOf(sql string) (Write, bool) {
	words := sqlWords(sql)
	i := 0
	if len(words) > 0 && strings.EqualFold(words[0], "WITH") {
		i = afterWith(words)
	}
	if i >= len(words) {
		return Write{}, false
	}
	op := strings.ToUpper(words[i])
	i++
	skip := func(keywords ...string) {
		for i < len(words) && slices.Contains(keywords, strings.ToUpper(words[i])) {
			i++
		}
	}
	switch op {
	case "INSERT":
		skip("LOW_PRIORITY", "DELAYED", "HIGH_PRIORITY", "IGNORE", "INTO")
	case "REPLACE":
		skip("LOW_PRIORITY", "DELAYED", "INTO")
	case "UPDATE":
		skip("LOW_PRIORITY", "IGNORE")
	case "DELETE":
		for i < len(words) && strings.ToUpper(words[i]) != "FROM" {
			i++
		}
		i++
	default:
		return Write{}, false
	}
	if i >= len(words) || !isName(words[i]) {
		return Write{}, false
	}
	return Write{Table: words[i], Op: op}, true
}

// afterWith returns the place in words, the words of a statement that starts with WITH, of the
// first INSERT, UPDATE, DELETE or REPLACE outside parentheses, or len(words) when there is none.
func afterWith(words []string) int {
	depth := 0
	for i, w := range words {
		switch {
		case w == "(":
			depth++
		case w == ")":
			depth--
		case depth == 0 && slices.Contains(writeOps, strings.ToUpper(w)):
			return i
		}
	}
	return len(words)
}

// sqlWords splits sql into words: names, which may be quoted with backquotes and qualified with a
// dot, and single characters of punctuation, a quoted string being one quote. It follows MySQL's
// lexing: it skips spaces, # and /* */ comments, and -- comments, where -- starts a comment only
// when the second dash is followed by a space, a control character or the end of the string.
func sqlWords(sql string) []string {
	var words []string
	for len(sql) > 0 {
		switch {
		case sql[0] == '#':
			_, sql, _ = strings.Cut(sql, "\n")
		case strings.HasPrefix(sql, "--") && (len(sql) == 2 || sql[2] <= ' '):
			_, sql, _ = strings.Cut(sql, "\n")
		case strings.HasPrefix(sql, "/*"):
			_, sql, _ = strings.Cut(sql, "*/")
		case strings.ContainsRune(" \t\r\n", rune(sql[0])):
			sql = sql[1:]
		case sql[0] == '\'' || sql[0] == '"':
			words = append(words, sql[:1])
			sql = afterString(sql)
		case isNameByte(sql[0]) || sql[0] == '`':
			var name string
			name, sql = sqlName(sql)
			words = append(words, name)
		default:
			words = append(words, sql[:1])
			sql = sql[1:]
		}
	}
	return words
}

// afterString returns the rest of sql after the quoted string at its start, whose quote is
// escaped inside it by a backslash or by doubling it.
func afterString(sql string) string {
	quote := sql[0]
	for i := 1; i < len(sql); i++ {
		switch {
		case sql[i] == '\\':
			i++
		case sql[i] == quote && i+1 < len(sql) && sql[i+1] == quote:
			i++
		case sql[i] == quote:
			return sql[i+1:]
		}
	}
	return ""
}

// sqlName reads the name at the start of sql, its parts joined by dots, and returns it without
// backquotes, with the rest of sql.
func sqlName(sql string) (string, string) {
	var parts []string
	for {
		var part string
		if strings.HasPrefix(sql, "`") {
			part, sql = backtickName(sql)
		} else {
			n := 0
			for n < len(sql) && isNameByte(sql[n]) {
				n++
			}
			part, sql = sql[:n], sql[n:]
		}
		parts = append(parts, part)
		if !strings.HasPrefix(sql, ".") || len(sql) < 2 || !isNameByte(sql[1]) && sql[1] != '`' {
			return strings.Join(parts, "."), sql
		}
		sql = sql[1:]
	}
}

// backtickName reads a `-quoted name at the start of sql, in which a doubled backquote is one
// literal backquote, and returns it without the surrounding quotes, with the rest of sql.
func backtickName(sql string) (string, string) {
	sql = sql[1:]
	var b strings.Builder
	for {
		j := strings.IndexByte(sql, '`')
		if j < 0 {
			b.WriteString(sql)
			return b.String(), ""
		}
		b.WriteString(sql[:j])
		if j+1 < len(sql) && sql[j+1] == '`' {
			b.WriteByte('`')
			sql = sql[j+2:]
			continue
		}
		return b.String(), sql[j+1:]
	}
}

// isName reports whether word is a name rather than punctuation.
func isName(word string) bool {
	return word != "" && (isNameByte(word[0]) || len(word) > 1)
}

// isNameByte reports whether c may be part of a name that is not quoted.
func isNameByte(c byte) bool {
	return c == '_' || c == '$' || '0' <= c && c <= '9' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || c >= 0x80
}
