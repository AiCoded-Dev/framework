# What cannot be built

AI assistants make the same few mistakes that leaked data from well-known AI-built apps. The
framework and the delivery pipeline refuse each one: with a code the builder sees, or before the
request reaches any of the app's own code.

Each case below says what goes wrong and what refuses it, and links the tests that make the
attempt on purpose, in small apps like those an AI assistant writes. Where a test also runs the
same app without the mistake, that app works, so the mistake alone is refused. `aicoded publish`
runs `aicoded check` before it sends an app, and the delivery pipeline runs it again on every
publish. The tests are part of the open framework, for anyone to read and run.

## A table anyone can read

**What goes wrong:** the rules that say who may see which data are missing or wrong, so anyone
with the link reads every row.

**What refuses it:**

- **A page with no access rule.** Every page is closed until an access rule on its path says who
  may open it. `aicoded check` refuses a list of every note that has no rule:
  [E-GEN-030](errors/E-GEN-030.md). Test:
  [`TestListWithNoAccessRule`](../cmd/aicoded/internal/cantbebuilt/open_table_test.go), app:
  [open-notes](../cmd/aicoded/internal/cantbebuilt/testdata/open-notes).
- **A page that opens any record by its id.** A page with an id in its URL must say whether its
  Guard decides who sees each record (`guard="true"`) or everyone its rule admits sees every
  record (`shared="true"`). `aicoded check` refuses a page that opens any note by the id in its
  URL and says neither: [E-GEN-054](errors/E-GEN-054.md). The same app, whose Guard lets in only
  the note's owner, passes the check. Test:
  [`TestNoteOpenedByAnyID`](../cmd/aicoded/internal/cantbebuilt/open_table_test.go), apps:
  [any-note](../cmd/aicoded/internal/cantbebuilt/testdata/any-note) and, with the Guard,
  [own-notes](../cmd/aicoded/internal/cantbebuilt/testdata/own-notes).
- **A page written by hand.** `aicoded generate` never writes a page without a rule. A list of
  every note written by hand instead, with no rule, is still refused when the app serves it: a
  signed-in viewer gets 403 and none of the notes, and the app logs
  [E-WEB-003](errors/E-WEB-003.md). The same page with a rule shows the notes. Test:
  [`TestPageServedWithoutAccessRule`](../cmd/aicoded/internal/cantbebuilt/open_table_test.go),
  which holds the page.

## A database key in the app

**What goes wrong:** an app that holds its database's address and password, or another key, hands
them to whoever gets its code.

**What refuses it:**

- **An app that opens its database itself.** An app may get its database only from the database
  building block, and `aicoded check` refuses any app that opens one itself, with or without a
  password, at the line that opens it: [E-LINT-006](errors/E-LINT-006.md). In the test, a notes
  app opens its database with an address that holds the user and the password, and is refused.
  The same app that gets its database from the building block, with no address and no password,
  passes. Test:
  [`TestDatabaseOpenedWithPassword`](../cmd/aicoded/internal/cantbebuilt/database_key_test.go),
  apps: [notes-dsn](../cmd/aicoded/internal/cantbebuilt/testdata/notes-dsn) and, with the
  building block, [own-notes](../cmd/aicoded/internal/cantbebuilt/testdata/own-notes).
- **A key written in the code.** The delivery pipeline's security checks look for passwords, keys
  and tokens in every commit that is published, and stop the publish on any they find:
  [E-GATE-018](errors/E-GATE-018.md). A test of the delivery pipeline, which is not public, makes
  the attempt with a key in an app's code.

**And the runner hands the app no key to leak.** The runner logs in to the database for the app.
In the test, an app whose permission list declares a database looks for a key where a runner
could put one: its only environment variable names the folder of its sockets, and that folder
holds nothing but sockets. Yet the app writes a row to its database and reads it back.
`aicoded check` would refuse this app's code, and the test runs it as if it had got past the
checks. Test: [`TestNothingToLeak`](../cmd/aicoded/internal/cantbebuilt/database_key_test.go),
app: [key-hunt](../cmd/aicoded/internal/cantbebuilt/testdata/key-hunt).

## A request that skips the login

**What goes wrong:** an app that trusts the browser, or forgets to check the session on one form,
lets in a request that never passed the login.

**What refuses it:** the app does not write this check. Every request carries a viewer token that
the runner signs for this one app, valid for at most a minute, and the framework refuses a request
without a valid one with 401 before it reaches any of the app's own code: a page, a form and a
live connection alike.

In the test, requests go straight to a notes page that only staff may open, past the company
login, with no token, a malformed, forged or expired one, or one for another app. Each gets 401
and nothing of the page, the app logs why, and the form adds no note. The same requests with a
token signed for a member of staff get the page, add the note and open the live connection. Test:
[`TestRequestThatSkipsTheLogin`](../cmd/aicoded/internal/cantbebuilt/skip_login_test.go), app:
[staff-notes](../cmd/aicoded/internal/cantbebuilt/testdata/staff-notes).
