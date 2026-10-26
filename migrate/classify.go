package migrate

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"aicoded.dev/framework/internal/errs"
)

// Kind is the kind of a statement that Classify allows.
type Kind int

// Expand is a statement that adds to the schema and changes or removes nothing in it.
const Expand Kind = 1

const (
	allowedFix = "write only CREATE TABLE, CREATE INDEX and ALTER TABLE … ADD statements on the app's own tables, " +
		"and change data from the app's code"
	destructiveFix = "make the change expand-only: add a new column or table and stop using the old one; " +
		"destructive changes need an approval, which comes later"
	journal = "aicoded_migrations"
)

// Check splits a draft into its statements and classifies each. It returns the statements and
// one error that joins every problem, each at the line of the draft its statement starts on.
func Check(draft []byte) ([]string, error) {
	stmts, err := split(draft)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(stmts))
	var problems []error
	for i, s := range stmts {
		out[i] = s.text
		if _, err := Classify(s.text); err != nil {
			var coded *errs.Error
			errors.As(err, &coded)
			problems = append(problems, at(s.line, coded))
		}
	}
	return out, errors.Join(problems...)
}

// Classify returns Expand for a statement that only adds to the schema of the app's own
// database: CREATE TABLE, CREATE INDEX, and ALTER TABLE with only ADD clauses. It refuses a
// destructive statement with E-MIG-003, and every other statement, and any it cannot read,
// with E-MIG-002.
func Classify(stmt string) (Kind, error) {
	toks, _, lexErr := lex(stmt)
	if lexErr != nil {
		return 0, lexErr
	}
	toks = slices.DeleteFunc(toks, func(t token) bool { return t.kind == comment })
	if n := len(toks); n > 0 && toks[n-1].is(";") {
		toks = toks[:n-1]
	}
	if len(toks) == 0 {
		return 0, notAllowed("an empty statement is not a schema change", allowedFix)
	}
	if err := readable(toks); err != nil {
		return 0, err
	}
	var err error
	switch first := toks[0].up(); first {
	case "CREATE":
		err = create(toks[1:])
	case "ALTER":
		err = alter(toks[1:])
	case "DROP", "RENAME", "TRUNCATE":
		err = destructive(first + " removes or renames what is there")
	default:
		err = notAllowed(fmt.Sprintf("%s statements are not allowed in a schema change", toks[0].text), allowedFix)
	}
	if err != nil {
		return 0, err
	}
	return Expand, nil
}

// readable refuses what hides SQL from the checks or reaches past the app's own tables.
func readable(toks []token) error {
	for i, t := range toks {
		switch {
		case t.kind == hidden:
			return notAllowed("an executable comment /*! … */ or a hint /*+ … */ hides SQL from the checks",
				"leave out /*! … */ and /*+ … */ and write plain statements")
		case t.is(";"):
			return notAllowed("the text holds more than one statement", "end each statement with ;")
		case t.kind == str && t.text[0] == '"':
			return notAllowed("a string in double quotes, which MySQL may read as a name", "write strings in single quotes and names in backquotes")
		case t.kind == str && strings.Contains(t.text, `\`):
			return notAllowed("a backslash in a string, which MySQL reads in two ways", "write a quote inside a string as '' and leave out backslashes")
		case t.is("."):
			return notAllowed("a name with a database or table in front of it", "name the app's own tables alone, without a database in front")
		case t.is("@") || t.is(`\`):
			return notAllowed("a variable or a client command", allowedFix)
		case i > 0 && t.up() == "SELECT":
			return notAllowed("a query inside a schema change", allowedFix)
		case t.isName() && strings.EqualFold(t.name(), journal):
			return notAllowed("the table "+journal+" is the platform's journal of migrations", "give the table another name")
		}
	}
	return nil
}

// create classifies the tokens after CREATE.
func create(toks []token) error {
	switch {
	case len(toks) > 0 && toks[0].up() == "TABLE":
		return createTable(toks[1:])
	case len(toks) > 0 && toks[0].up() == "INDEX":
		return createIndex(toks[1:])
	case len(toks) > 1 && slices.Contains([]string{"UNIQUE", "FULLTEXT", "SPATIAL"}, toks[0].up()) && toks[1].up() == "INDEX":
		return createIndex(toks[2:])
	case len(toks) > 0:
		return notAllowed(fmt.Sprintf("CREATE %s is not allowed in a schema change", toks[0].text), allowedFix)
	}
	return notAllowed("CREATE without what it creates", allowedFix)
}

// createTable classifies the tokens after CREATE TABLE: a name, the columns in parentheses and
// table options.
func createTable(toks []token) error {
	if len(toks) < 3 || !toks[0].isName() || !toks[1].is("(") || toks[2].up() == "LIKE" {
		return notAllowed("CREATE TABLE must name one table and list its columns in parentheses; "+
			"IF NOT EXISTS, TEMPORARY, LIKE and AS SELECT are not allowed", allowedFix)
	}
	end, err := closing(toks, 1)
	if err != nil {
		return err
	}
	return tableOptions(toks[end+1:])
}

// tableOptions allows ENGINE=InnoDB, the character set, the collation, a comment, the first
// AUTO_INCREMENT value and the row format, and refuses every other table option.
func tableOptions(toks []token) error {
	value := func(i int) int {
		if i < len(toks) && toks[i].is("=") {
			i++
		}
		return i + 1
	}
	for i := 0; i < len(toks); {
		switch t := toks[i]; {
		case t.is(","):
			i++
		case t.up() == "DEFAULT" && i+1 < len(toks) && slices.Contains([]string{"CHARSET", "CHARACTER", "COLLATE"}, toks[i+1].up()):
			i++
		case t.up() == "CHARACTER" && i+1 < len(toks) && toks[i+1].up() == "SET":
			i = value(i + 2)
		case t.up() == "ENGINE":
			i = value(i + 1)
			if i > len(toks) || !strings.EqualFold(toks[i-1].name(), "InnoDB") {
				return notAllowed("a table's ENGINE must be InnoDB", "leave out ENGINE, or write ENGINE=InnoDB")
			}
		case slices.Contains([]string{"CHARSET", "COLLATE", "COMMENT", "AUTO_INCREMENT", "ROW_FORMAT"}, t.up()):
			i = value(i + 1)
		default:
			return notAllowed(fmt.Sprintf("the table option %s is not allowed", t.text),
				"keep to ENGINE=InnoDB, CHARSET, COLLATE, COMMENT, AUTO_INCREMENT and ROW_FORMAT after the columns")
		}
	}
	return nil
}

// createIndex classifies the tokens after CREATE … INDEX: a name, ON, a table and the columns
// in parentheses.
func createIndex(toks []token) error {
	shape := notAllowed("CREATE INDEX must be CREATE INDEX <name> ON <table> (<columns>), with nothing after the columns", allowedFix)
	if len(toks) < 4 || !toks[0].isName() || toks[1].up() != "ON" || !toks[2].isName() || !toks[3].is("(") {
		return shape
	}
	end, err := closing(toks, 3)
	if err != nil {
		return err
	}
	if end != len(toks)-1 {
		return shape
	}
	return nil
}

// alter classifies the tokens after ALTER: TABLE, a name and clauses. A destructive clause
// decides; otherwise every clause must add.
func alter(toks []token) error {
	if len(toks) < 3 || toks[0].up() != "TABLE" || !toks[1].isName() {
		return notAllowed("only ALTER TABLE <table> ADD … is allowed", allowedFix)
	}
	clauses, err := clauses(toks[2:])
	if err != nil {
		return err
	}
	for _, c := range clauses {
		if w := destroys(c); w != "" {
			return destructive("ALTER TABLE … " + w + " changes or removes what is there")
		}
	}
	for _, c := range clauses {
		if err := add(c); err != nil {
			return err
		}
	}
	return nil
}

// clauses splits toks at the commas outside parentheses.
func clauses(toks []token) ([][]token, error) {
	var out [][]token
	depth, from := 0, 0
	for i, t := range toks {
		switch {
		case t.is("("):
			depth++
		case t.is(")"):
			depth--
		case t.is(",") && depth == 0:
			out = append(out, toks[from:i])
			from = i + 1
		}
		if depth < 0 {
			return nil, unbalanced()
		}
	}
	if depth != 0 {
		return nil, unbalanced()
	}
	out = append(out, toks[from:])
	for _, c := range out {
		if len(c) == 0 {
			return nil, notAllowed("ALTER TABLE has an empty clause", "write a clause between each two commas, and none after the last")
		}
	}
	return out, nil
}

// destroys returns the word that makes a clause of ALTER TABLE destructive, or "".
func destroys(clause []token) string {
	if clause[0].up() == "ALTER" {
		return "ALTER"
	}
	for _, t := range outside(clause) {
		if w := t.up(); slices.Contains([]string{"DROP", "MODIFY", "CHANGE", "RENAME", "TRUNCATE"}, w) {
			return w
		}
	}
	return ""
}

// add allows a clause of ALTER TABLE that adds a column, an index, or a primary key, unique
// or foreign key constraint.
func add(clause []token) error {
	if clause[0].up() != "ADD" {
		return notAllowed(fmt.Sprintf("ALTER TABLE … %s is not allowed: only ADD clauses are", clause[0].text), allowedFix)
	}
	for _, t := range outside(clause) {
		if slices.Contains([]string{"PARTITION", "PARTITIONS", "PARTITIONING"}, t.up()) {
			return notAllowed("partitions are not allowed in a schema change", allowedFix)
		}
	}
	rest := clause[1:]
	next := func(i int) string {
		if i < len(rest) {
			return rest[i].up()
		}
		return ""
	}
	key := func(i int) bool { return next(i) == "INDEX" || next(i) == "KEY" }
	switch next(0) {
	case "COLUMN":
		return column(rest[1:])
	case "INDEX", "KEY", "UNIQUE":
		return nil
	case "FULLTEXT", "SPATIAL":
		if key(1) {
			return nil
		}
	case "PRIMARY", "FOREIGN":
		if next(1) == "KEY" {
			return nil
		}
	case "CONSTRAINT":
		i := 1
		if !slices.Contains([]string{"PRIMARY", "FOREIGN", "UNIQUE", "CHECK"}, next(1)) && i < len(rest) && rest[i].isName() {
			i++
		}
		if next(i) == "UNIQUE" || (next(i) == "PRIMARY" || next(i) == "FOREIGN") && next(i+1) == "KEY" {
			return nil
		}
	case "CHECK":
	default:
		return column(rest)
	}
	return notAllowed("ALTER TABLE … ADD may add a column, an index, or a PRIMARY KEY, UNIQUE or FOREIGN KEY constraint", allowedFix)
}

// column allows the definition of a new column that is NULL or has a DEFAULT.
func column(def []token) error {
	if len(def) < 2 || !def[0].isName() || def[1].kind != word {
		return notAllowed("ADD COLUMN must name one column and its type", allowedFix)
	}
	var notNull, primary, dflt bool
	attrs := outside(def[2:])
	for i, t := range attrs {
		nextIs := func(w string) bool { return i+1 < len(attrs) && attrs[i+1].up() == w }
		switch t.up() {
		case "NOT":
			notNull = notNull || nextIs("NULL")
		case "PRIMARY":
			primary = primary || nextIs("KEY")
		case "DEFAULT":
			dflt = true
		}
	}
	if (notNull || primary) && !dflt {
		return notAllowed(fmt.Sprintf("the new column %s is NOT NULL without a DEFAULT, so the version of the app that runs now cannot add rows", def[0].name()),
			"give the column a DEFAULT, or let it be NULL")
	}
	return nil
}

// outside returns the tokens of toks outside parentheses.
func outside(toks []token) []token {
	var out []token
	depth := 0
	for _, t := range toks {
		switch {
		case t.is("("):
			depth++
		case t.is(")"):
			depth--
		case depth == 0:
			out = append(out, t)
		}
	}
	return out
}

// closing returns the index of the parenthesis that closes toks[open].
func closing(toks []token, open int) (int, error) {
	depth := 0
	for i := open; i < len(toks); i++ {
		switch {
		case toks[i].is("("):
			depth++
		case toks[i].is(")"):
			depth--
			if depth == 0 {
				return i, nil
			}
		}
	}
	return 0, unbalanced()
}

func unbalanced() error {
	return notAllowed("a parenthesis is not closed or not opened", "close every parenthesis the statement opens")
}

func notAllowed(msg, fix string) error {
	return errs.New("E-MIG-002", msg, fix)
}

func destructive(msg string) error {
	return errs.New("E-MIG-003", msg, destructiveFix)
}
