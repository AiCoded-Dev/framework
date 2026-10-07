# Examples

Three reference apps built with the framework: people, a staff directory with pages; contacts,
which serves people's phone numbers and email addresses to it; and room-maintenance, where hotel
staff report problems in rooms, each of them sees only their own tickets, and hotel-ops see all.

Copy what they do: every page has an access rule, ownership is checked by the page and again where
the data is, no form value reaches a log, mail carries an idempotency key, and SQL passes values
with `?` placeholders.

Each app is its own Go module and uses this checkout of the framework through a `replace` line,
which `aicoded check` accepts only from an `aicoded` installed from this checkout (E-LINT-011).
The delivery pipeline refuses a `replace` line, so an app of your own requires a released
framework version instead.

## Running them

You need a MySQL 8 server on this machine, `aicoded` installed from this checkout with
`make install`, and a browser. Safari does not work with `aicoded dev`, as
[aicoded dev](../docs/guides/dev.md) explains.

1. Add the examples' folder to `dev.yaml` in your user configuration directory
   (`~/.config/aicoded/dev.yaml` on Linux, `~/Library/Application Support/aicoded/dev.yaml` on
   macOS, with mode 600): the admin DSN of the MySQL server, the personas and the team's address.

   ```yaml
   workspaces:
     /absolute/path/to/framework/examples:
       mysql: root:<password>@unix(/run/mysqld/mysqld.sock)/
       personas:
         - {name: alice, roles: [staff]}
         - {name: bob, roles: [staff, hr]}
         - {name: carol}
         - {name: dana, roles: [staff, hotel-ops]}
       apps:
         people:
           settings: {team_address: team@acme.example}
   ```

2. Run `aicoded dev examples` in the checkout. It starts contacts first, since people calls it.
3. Open `http://people.localhost:8080/` or `http://room-maintenance.localhost:8080/`, and switch
   personas at `/_aicoded/persona` on either, such as
   `http://people.localhost:8080/_aicoded/persona`.
4. Read the mail people sends on the Mail page of the dev UI; `aicoded dev` prints its login link.

| Persona | Roles | In people | In room-maintenance |
|---|---|---|---|
| alice | staff | everything carol can, list the users, and read her own contacts | report problems, and open and edit her own tickets |
| bob | staff, hr | everything alice can, add users, and read anyone's contacts | the same as alice, with tickets of his own |
| carol | none | open the home page and send the contact form | nothing: every page answers 403 |
| dana | staff, hotel-ops | everything carol can, and list the users | everything alice can, open every ticket and change its status |

people and contacts start with the same twelve people, `alice` and `bob` among them.

## people

people is a staff directory. Its pages:

| Page | Access rule | What it shows |
|---|---|---|
| `/home` | every viewer | the template language: arithmetic, conditions, loops, markup made with `web.HTMLConst`, `web.EscapeHTML` and `web.JoinHTML`, an image, data passed to a script with `<ssr:json>`, the live number of open home pages, and a display name the page writes back and the server checks |
| `/contact` | every viewer | a form that mails the team at the setting `team_address` |
| `/users` | `staff` | the users from the SQL database and, live, how many there are; it opens the first user |
| `/users/add` | `staff`, then `hr` | a form with every kind of field: text, a number, a select with groups and a closed option, a multiple select, checkboxes, radio buttons, a text area, a photo and more photos |
| `/users/<login>/info` | `staff` | a user's details and, live, when they last opened a page |
| `/users/<login>/contacts` | `staff`, then a `Guard` | the user's phones and emails, from contacts |

- `deps.Deps` in [people/deps/deps.go](people/deps/deps.go), which `main` passes to `app.Main` as
  `OnStart`, opens the database and the photo store, creates and fills the `users` table, and
  reads `team_address`. Its hubs carry live values from page to page: a `reactive.Broadcast` for
  the open home pages and one for added users, and a `reactive.Topic` keyed by login for when a
  viewer last opened a page.
- Adding a user inserts it in a transaction that commits only once its photos are stored, inside
  a span of its own, and only then mails the team with the idempotency key `user-<login>`. Photos
  go to the file store `photos` as `<login>/<file name>`, and an add that fails removes the photos
  it stored. If the mail fails after the commit, the user exists and the mail is not retried: the
  form sent again answers "That login is taken." Serving a stored file from a page comes later, so
  the user's card shows each photo's name and size, and initials instead of a picture.
- The contact form mails the team with an idempotency key made from the viewer, the topic and the
  message, so a message sent twice is mailed once. It stores and logs nothing.
- The contacts tab's `Guard` lets a viewer see their own contacts, and a viewer with `hr` anyone's.
  Its `Data` calls `contacts.Contacts` with the request's context, so the call goes on behalf of
  the viewer, and answers 403 for `rpc.PermissionDenied` and 404 for `rpc.NotFound`. A user added
  in people has no contacts in contacts, so their tab answers 404.
- A login is 1 to 32 lowercase letters and digits. The `login` column compares exactly, with no
  case folding and no trailing-space padding, so `Alice` and `alice ` are not `alice`.

Its tests check the add form's checks of the login and the photos, the contact form's address
check and idempotency key, and the card's initials without a runner. They check the database code
against the MySQL server that `AICODED_TEST_MYSQL_DSN` names, skipping without it, with the photo
store in memory. They open that server with the driver `github.com/go-sql-driver/mysql`: app code
never imports a database driver, but until a test kit exists a test may open the MySQL server that
`AICODED_TEST_MYSQL_DSN` names.

## contacts

contacts has no pages. It keeps the contacts in its own SQL database: `deps.Start`, which `main`
passes to `app.Main` as `OnStart`, opens the database, creates the `contacts` table and, when the
table is empty, fills it with sample people. The app serves one function, in
[contacts/rpc/rpc.go](contacts/rpc/rpc.go):

```go
//ssr:access caller=people role=staff
func Contacts(ctx context.Context, in ContactsIn) (ContactsOut, error)
```

- Only the app `people` may call it, on behalf of a viewer with the `staff` role.
- The viewer gets the contacts of their own login, or anyone's with the `hr` role. Anyone else
  gets `rpc.PermissionDenied`. The calling page checks the same rule in its `Guard`, and contacts
  checks it again because it holds the data.
- A login with no contacts gets `rpc.NotFound`.
- Logins match exactly: the `login` column compares with no case folding and no trailing-space
  padding, so `Alice` and `alice ` are not `alice`.

Its tests check the rule, and that `Contacts` checks it before it reads the database, without a
runner, and the database code against the MySQL server that `AICODED_TEST_MYSQL_DSN` names;
without the variable those tests are skipped. They open that server as people's tests do.

## room-maintenance

room-maintenance is the plainest picture of records with an owner. Any employee with the `staff`
role reports a problem in a room, and sees and edits only their own tickets; hotel-ops see every
ticket and change its status, which shows live in every open list that holds the ticket. Its
[README](room-maintenance/README.md) shows where each rule is checked: the access rules, a `Guard`
on each ticket, the SQL that reads and changes tickets, and the hubs that wake the lists.

## Checks

`make check` in the checkout tests, lints and scans every example like the framework itself, and
`TestExamplesAreGenerated` fails when an example's generated files are out of date.
`aicoded check --frozen examples` checks every example without writing anything: it reports
generated files that are out of date, and builds, vets, lints and tests each app.

The end-to-end tests in `cmd/aicoded/internal/e2e` run the three examples under `aicoded dev` and
drive people and room-maintenance in Chrome. They drop the `ac-dev-contacts`, `ac-dev-people` and
`ac-dev-room-maintenance` databases and MySQL users, so `AICODED_TEST_MYSQL_DSN` must point at a
throwaway server, not the one your own `dev.yaml` uses.
