# List rows from the database

Show a list read from the app's own SQL database. The example is the list of users in the people
example, at `/users`: its `deps` package holds the table and the queries, and the page shows the
rows.

## Steps

1. Declare the database in the permission list, under `data`: `- source: sqldb` with
   `classes: [internal]`. Open it once, in `Deps.Start`, with `sqldb.Open(ctx)`, as
   [Start a new app](new-app.md) shows. There is no DSN: the runner logs in as the app's own
   database user.
2. Create the table in `Start`, before the app serves. people creates `users` and, when it is
   empty, fills it in one transaction:

   <!-- code: examples/people/deps/users.go prepare -->
   ```go
   // prepare creates the users table and, when it is empty, fills it with seed. A login compares
   // exactly, with no case folding and no trailing-space padding, so "Alice" and "alice " are not
   // "alice".
   func prepare(ctx context.Context, db *sql.DB) error {
   	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS users (
   		id BIGINT AUTO_INCREMENT PRIMARY KEY,
   		login VARCHAR(32) COLLATE utf8mb4_0900_bin NOT NULL UNIQUE,
   		name VARCHAR(100) NOT NULL,
   		age TINYINT UNSIGNED NOT NULL,
   		team VARCHAR(40) NOT NULL,
   		languages VARCHAR(100) NOT NULL,
   		on_site BOOL NOT NULL,
   		remote BOOL NOT NULL,
   		shift TINYINT UNSIGNED NOT NULL,
   		bio TEXT NOT NULL
   	)`); err != nil {
   		return err
   	}
   	tx, err := db.BeginTx(ctx, nil)
   	if err != nil {
   		return err
   	}
   	defer func() { _ = tx.Rollback() }()
   	var n int
   	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&n); err != nil {
   		return err
   	}
   	if n > 0 {
   		return nil
   	}
   	for _, u := range seed {
   		if err := insert(ctx, tx, u); err != nil {
   			return err
   		}
   	}
   	return tx.Commit()
   }
   ```

   <!-- code: examples/people/deps/users.go insert -->
   ```go
   // insert adds u to the users table.
   func insert(ctx context.Context, tx *sql.Tx, u User) error {
   	_, err := tx.ExecContext(ctx, "INSERT INTO users ("+columns+") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
   		u.Login, u.Name, u.Age, u.Team, u.Languages, u.OnSite, u.Remote, u.Shift, u.Bio)
   	return err
   }
   ```

   Values always go into the statement with `?` placeholders, never into its text. One
   statement runs per call.
3. Read the rows in `deps`, into a type of their own:

   <!-- code: examples/people/deps/users.go User -->
   ```go
   // User is one person in the users table.
   type User struct {
   	Login string
   	Name  string
   	Age   int
   	Team  string
   	// Languages lists the languages the person speaks, joined with ", ".
   	Languages string
   	OnSite    bool
   	Remote    bool
   	// Shift is 1 for the day shift and 2 for the night shift.
   	Shift int
   	Bio   string
   }
   ```

   <!-- code: examples/people/deps/users.go columns -->
   ```go
   const columns = "login, name, age, team, languages, on_site, remote, shift, bio"
   ```

   <!-- code: examples/people/deps/users.go Deps.Users -->
   ```go
   // Users returns every user, in the order they were added.
   func (d *Deps) Users(ctx context.Context) ([]User, error) {
   	rows, err := d.DB.QueryContext(ctx, "SELECT "+columns+" FROM users ORDER BY id")
   	if err != nil {
   		return nil, err
   	}
   	defer rows.Close()
   	var users []User
   	for rows.Next() {
   		u, err := scan(rows)
   		if err != nil {
   			return nil, err
   		}
   		users = append(users, u)
   	}
   	return users, rows.Err()
   }
   ```

   <!-- code: examples/people/deps/users.go scan -->
   ```go
   // scan reads a user from a row that holds the columns.
   func scan(row interface{ Scan(...any) error }) (User, error) {
   	var u User
   	err := row.Scan(&u.Login, &u.Name, &u.Age, &u.Team, &u.Languages, &u.OnSite, &u.Remote, &u.Shift, &u.Bio)
   	return u, err
   }
   ```

4. Name the row's type in the page's package, in a `.go` file next to the template:

   <!-- code: examples/people/pages/users/types.go -->
   ```go
   package users

   import "people/deps"

   // User is a user of the list.
   type User = deps.User
   ```

5. Show the rows with `ssr:for`. This page is also the layout of the user pages: the list stays,
   and `<ssr:content/>` shows the user that is open.

   <!-- code: examples/people/pages/users/index.html -->
   ```html
   <ssr:access role="staff"/>
   <ssr:var name="users" type="[]User"/>
   <ssr:var name="current" type="string"/>
   <ssr:var name="canAdd" type="bool"/>
   <ssr:var name="userCount" type="int" reactive="true"/>
   <div class="users">
     <aside>
       <h2>Users</h2>
       <p class="muted"><span id="user-count">{{ userCount }}</span> people</p>
       <a id="add-user" ssr:if="canAdd" href="/users/add">Add a user</a>
       <ul id="user-list">
         <li ssr:for="u in users"><a class="{{ u.Login == current ? 'active' : '' }}" href="/users/{{ u.Login }}">{{ u.Name }}</a></li>
       </ul>
     </aside>
     <section>
       <ssr:content/>
     </section>
   </div>
   ```

6. Fill the list in `Data`:

   <!-- code: examples/people/pages/users/dataprovider.go DP.Data -->
   ```go
   // Data lists the users, marks the one open, and counts them.
   func (p *DP) Data(ctx context.Context, r *web.Request, _ web.ResponseWriter, data *RouteData) error {
   	users, err := p.d.Users(ctx)
   	if err != nil {
   		return err
   	}
   	data.Users = users
   	data.Current = r.URLParam("login")
   	data.CanAdd = auth.Viewer(ctx).HasRole("hr")
   	data.UserCount = len(users)
   	return nil
   }
   ```

## Check it

- `aicoded check` generates, builds, vets, lints and tests the app.
- `aicoded dev` makes the app's own database, `ac-dev-people`, on the MySQL server that
  `dev.yaml` names. Open `http://people.localhost:8080/users` as a persona with the `staff` role.
- The dev UI's Traces page shows each statement as a span, such as `sql.query`, which records
  only the statement's first word, never its text or its values.

## See also

- [SQL database](../guides/sql-database.md)
- [Template syntax](../guides/template-syntax.md)
- [The permission list](../guides/permission-list.md)
