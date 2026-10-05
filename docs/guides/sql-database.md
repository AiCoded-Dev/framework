# The SQL database

`sqldb.Open(ctx)` returns the app's own MySQL database as a `*sql.DB`. There is no DSN and no
password: the app talks to its runner, which logs in to the database as the app's own user,
whose grants reach only the app's database.

## Declaring and opening it

Declare the database in the permission list:

```yaml
data:
  - source: sqldb
    classes: [internal]
```

Without it, `sqldb.Open` fails with E-MAN-010. App code never opens a database itself:
`aicoded check` refuses `sql.Open`, `sql.OpenDB`, `sql.Register` and `db.Driver()`
(E-LINT-006). Open the database once, in `OnStart`, and share it with the pages through
`deps.Deps`. The people example opens it in `Start`, which `main` passes to `app.Main` as
`OnStart`:

<!-- code: examples/people/deps/deps.go Deps.Start -->
```go
// Start opens the database and the photo store, prepares the users table and reads the team's
// address.
func (d *Deps) Start(ctx context.Context) error {
	db, err := sqldb.Open(ctx)
	if err != nil {
		return err
	}
	if err := prepare(ctx, db); err != nil {
		return err
	}
	photos, err := filestore.Open(ctx, "photos")
	if err != nil {
		return err
	}
	team, err := config.String(ctx, "team_address")
	if err != nil {
		return err
	}
	d.DB, d.Photos, d.TeamAddress = db, photos, team
	return nil
}
```

```go
// main.go
func main() {
	d := deps.New()
	app.Main(app.Options{Handler: pages.NewHandler(d), OnStart: d.Start})
}
```

`prepare` creates the table with `CREATE TABLE IF NOT EXISTS` and, in one transaction, fills it
with sample rows when it is empty.

## Queries

- Write every statement as a constant: a string literal, a constant, or constants joined with
  `+`. `aicoded check` refuses SQL built at run time, from a variable, with `fmt.Sprintf` or
  from a call with several results, SQL with an executable comment such as `/*! ... */`, more
  than one statement in a call, and `INTO OUTFILE`, `INTO DUMPFILE` or `LOAD_FILE`, in Go code
  and in a template's expressions alike (E-LINT-007). When the viewer's role decides which rows
  to read, write one constant statement for each case and choose one in Go, as
  [List own records, or all of them for a role](../tasks/list-by-role.md) shows.
- Pass every value with a `?` placeholder, never by joining it into the SQL text.
- One statement runs per call, and statements with arguments are always prepared.
- Times are read and written in UTC.
- Pass the request's `ctx`, so a query ends when the request does and shows in its trace.

<!-- code: examples/people/deps/users.go Deps.User -->
```go
// User returns the user with login, or ErrNoUser.
func (d *Deps) User(ctx context.Context, login string) (User, error) {
	u, err := scan(d.DB.QueryRowContext(ctx, "SELECT "+columns+" FROM users WHERE login = ?", login))
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNoUser
	}
	return u, err
}
```

The page then turns `ErrNoUser` into `web.NotFound()`.

- Close every `*sql.Rows`, with `defer rows.Close()`, and check `rows.Err()` after the loop.
  The `*sql.DB` that `sqldb.Open` returns keeps at most 16 connections, and a query waits for a
  free one, so rows never closed make later requests wait. The runner refuses more than 32
  connections from one app (E-SQL-004), which only a second `*sql.DB` or a raised pool limit
  can reach.
- Use a transaction, `db.BeginTx` with `defer tx.Rollback()` and `tx.Commit()` at the end, for
  writes that belong together. `Deps.AddUser` in the people example checks the login, inserts the
  user and stores the photos in one, and commits only when all of them worked.
- A column that an access check compares, such as a login, needs a collation that compares
  exactly, with no case folding and no trailing-space padding: `utf8mb4_0900_bin`. Under MySQL's
  default collation `Alice` equals `alice`, and even `utf8mb4_bin` treats `alice ` as `alice`, so
  a check that compares exactly in Go and a query that does not would disagree.
- Prefer a check for an existing row in a transaction over `INSERT IGNORE`, which also turns
  truncated values and other errors into warnings. Keep a UNIQUE index as the real guard: two
  transactions can both pass the check, and the index then refuses the second insert with an
  error that names the value. `Deps.AddUser` checks again after such an error, outside its
  transaction, and returns `ErrLoginTaken`.

## A list of values

Since the SQL is a constant, a list of values for `IN` cannot be written into it. Pass the list
as one JSON array through one placeholder, and let MySQL's `JSON_TABLE` turn it into rows:

```go
logins, err := json.Marshal(list) // list is a []string
if err != nil {
	return err
}
rows, err := d.DB.QueryContext(ctx, `SELECT login, name FROM users
	WHERE login IN (SELECT login FROM JSON_TABLE(?, '$[*]' COLUMNS (login VARCHAR(64) PATH '$')) AS l)`,
	string(logins))
```

Give the JSON column the type of the column it is compared with. An empty or nil list matches no
row.

## What the runner refuses

The connection runs queries and prepared statements only. Changing the database user or the
connection's options, replication commands and other administrative commands are refused, and
the connection ends (E-SQL-001). The runner needs no TLS on it, since it never leaves the
machine, and refuses a client that asks for TLS or speaks an old protocol (E-SQL-003);
`sqldb.Open` sets the client up correctly. When the runner cannot log in to the database, the
app gets E-SQL-002.

## Traces

Every statement is a span: `sql.query`, `sql.exec` or `sql.begin`, with the attribute
`db.operation`, the statement's first word, such as `SELECT`. Outside `aicoded dev`, a failed
statement records the MySQL error number and state, such as `MySQL error 1062 (23000)`, or the
kind of any other error, never the statement or its values. See [telemetry](telemetry.md).

## In aicoded dev

An app that declares `sqldb` needs `mysql:` in its workspace in `dev.yaml`: the admin DSN of a
MySQL 8 server on this machine, over a Unix socket, `127.0.0.1` or `::1` (E-DEV-006,
E-DEV-007).

```yaml
workspaces:
  /absolute/path/to/apps:
    mysql: root:<password>@unix(/run/mysqld/mysqld.sock)/
```

`aicoded dev` creates the app's own database and user there, both named `ac-dev-<app>`, and
gives the user a new password on every start. The admin user needs `CREATE USER`, and
`ALL PRIVILEGES … WITH GRANT OPTION` on the apps' databases, including `CREATE` to create them
(E-DEV-008). See [aicoded dev](dev.md).

## See also

- [The permission list](permission-list.md): the `data` section.
- [people/deps/users.go](../../examples/people/deps/users.go): queries, a transaction and the
  table.
- [contacts/deps/deps.go](../../examples/contacts/deps/deps.go): a database used by an app with
  no pages.
- [List rows from the database](../tasks/list-from-database.md): a table, a query with
  placeholders and a list.
