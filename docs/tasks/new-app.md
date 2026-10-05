# Start a new app

Create an app with `aicoded init`, declare in its permission list what it reaches, open its
building blocks once in a `deps` package that every page shares, and run it under
`aicoded dev`. The code is that of the people example, a staff directory, and of contacts, an
app with no pages.

## Steps

1. In the folder that holds your apps, run `aicoded init people`. It makes the folder `people/`
   with `go.mod`, the permission list `aicoded.yaml`, `main.go`, one page, `pages/index.html`, and
   `AGENTS.md`, the rules for AI assistants that build the app. It then runs `aicoded generate`
   and `go mod tidy`. The name is 2 to 63 lowercase letters, digits and dashes, starting with a
   letter and ending with a letter or digit (E-CLI-002).
2. Write in `aicoded.yaml` what the app reaches. The finished people app has its own SQL
   database, a file store named `photos`, mail to its company's domain, and one setting, the
   team's address:

   <!-- code: examples/people/aicoded.yaml -->
   ```yaml
   app: people
   data:
     - source: sqldb
       classes: [internal]
     - source: filestore:photos
       classes: [internal]
   email:
     from: people@acme.example
     to_domains: [acme.example]
   settings: [team_address]
   # access is written by aicoded generate from <ssr:access>; do not edit it
   access:
     "/contact":
       require: ["*"]
     "/home":
       require: ["*"]
     "/users/add":
       require: ["staff", "hr"]
     "/users/{login}/contacts":
       require: ["staff"]
       guard: true
     "/users/{login}/info":
       require: ["staff"]
   # services is written by aicoded generate from rpc/ and the clients in services/; do not edit it
   services:
     calls:
       "contacts": ["Contacts"]
   ```

   You write `app`, `data`, `email`, `settings` and `secrets`, so copy only those sections.
   `aicoded generate` writes the `access` section from the pages and the `services` section from
   the calls between apps; never edit those two.
3. Make `deps/deps.go`, package `deps`, with a type `Deps` for what every page shares and a
   `Start` that opens the building blocks before the app serves:

   <!-- code: examples/people/deps/deps.go Deps -->
   ```go
   // Deps is made by New and filled by Start before the app serves.
   type Deps struct {
   	DB     *sql.DB
   	Photos *filestore.Store
   	// TeamAddress is the setting team_address, where the app mails the team.
   	TeamAddress string
   	// HomePages has one subscription per open home page, and a signal whenever one opens or
   	// closes.
   	HomePages *reactive.Broadcast[struct{}]
   	// UsersAdded signals that a user was added.
   	UsersAdded *reactive.Broadcast[struct{}]
   	// Seen records when each viewer last opened a page.
   	Seen *LastSeen
   }
   ```

   <!-- code: examples/people/deps/deps.go New -->
   ```go
   // New returns Deps with its hubs.
   func New() *Deps {
   	return &Deps{
   		HomePages:  reactive.NewBroadcast[struct{}](),
   		UsersAdded: reactive.NewBroadcast[struct{}](),
   		Seen:       NewLastSeen(),
   	}
   }
   ```

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

   `prepare` comes from [List rows from the database](list-from-database.md), and `LastSeen` and
   `NewLastSeen` from [Add a live value](add-live-value.md): leave them and the live hubs,
   `HomePages`, `UsersAdded` and `Seen`, out until you need them.

   There is no DSN and no password. `sqldb.Open`, `filestore.Open` and `config.String` reach,
   through the runner, only what the permission list declares, and refuse anything else
   (E-MAN-010, E-MAN-011, E-MAN-001).
4. In `main.go`, pass `Start` to `app.Main` as `OnStart`, and the `Deps` to the pages:

   <!-- code: examples/people/main.go -->
   ```go
   // People is a staff directory: staff look up their colleagues, and hr adds new ones.
   package main

   import (
   	"aicoded.dev/framework/app"

   	"people/deps"
   	"people/pages"
   )

   func main() {
   	d := deps.New()
   	app.Main(app.Options{Handler: pages.NewHandler(d), OnStart: d.Start})
   }
   ```

5. Once `deps` declares `Deps`, the `pages.NewHandler` that `aicoded generate` writes takes a
   `*deps.Deps` and passes it to the `NewDP` of every page. The page that `aicoded init` made
   still has `NewDP()`: delete `pages/dataprovider.go`, which holds no code of yours yet, and run
   `aicoded generate`, which writes it again with the `*deps.Deps`.
6. Give `aicoded dev` the local values in `dev.yaml`, in your user configuration directory
   (`~/.config/aicoded/dev.yaml` on Linux, `~/Library/Application Support/aicoded/dev.yaml` on
   macOS), with mode 600. The workspace is the absolute path of the folder you run `aicoded dev`
   in. An app with `sqldb` needs the admin DSN of a MySQL 8 server on this machine, and every
   setting needs a test value:

   ```yaml
   workspaces:
     /absolute/path/to/apps:
       mysql: root:<password>@unix(/run/mysqld/mysqld.sock)/
       personas:
         - {name: alice, roles: [staff]}
         - {name: bob, roles: [staff, hr]}
         - {name: carol}
       apps:
         people:
           settings: {team_address: team@acme.example}
   ```

7. Run `aicoded dev` in that folder and open `http://people.localhost:8080/` in Chrome. Switch
   personas at `http://people.localhost:8080/_aicoded/persona`.

## An app with no pages

contacts serves a function to people and has no pages. Its `main` passes only the generated
server of its `rpc/` package, and its `Start` is a plain function:

<!-- code: examples/contacts/main.go -->
```go
// Contacts serves the phone numbers and email addresses of people to the people app.
package main

import (
	"aicoded.dev/framework/app"

	"contacts/deps"
	"contacts/rpc"
)

func main() {
	app.Main(app.Options{RPC: rpc.Server(), OnStart: deps.Start})
}
```

<!-- code: examples/contacts/deps/deps.go Start -->
```go
// Start opens the app's database and prepares the contacts table.
func Start(ctx context.Context) error {
	conn, err := sqldb.Open(ctx)
	if err != nil {
		return err
	}
	if err := prepare(ctx, conn); err != nil {
		return err
	}
	db = conn
	return nil
}
```

Its permission list declares only its database. `aicoded generate` adds `access: {}` and the
function it serves:

<!-- code: examples/contacts/aicoded.yaml -->
```yaml
app: contacts
data:
  - source: sqldb
    classes: [internal]
# access is written by aicoded generate from <ssr:access>; do not edit it
access: {}
# services is written by aicoded generate from rpc/ and the clients in services/; do not edit it
services:
  serves:
    "Contacts":
      callers: ["people"]
      require: ["staff"]
```

## Check it

- `aicoded check` in the folder that holds your apps generates, builds, vets, lints and tests
  every app, and ends with `aicoded check: ok`. Each problem comes with its code, `file:line`
  and fix.
- `aicoded dev` prints a login link to its dev UI. Its Apps page shows people `running`, or
  `failed` with its problems: without `team_address` in `dev.yaml`, people fails with E-DEV-003.

## See also

- [Quick start](../guides/quickstart.md)
- [Project structure](../guides/project-structure.md)
- [The permission list](../guides/permission-list.md)
- [Settings and secrets](../guides/settings-and-secrets.md)
- [aicoded dev](../guides/dev.md)
- [Tools](../guides/tools.md)
