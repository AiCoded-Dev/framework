# Simulated attacks: how the delivery pipeline tests the running app

Once an app's tests pass, the delivery pipeline starts the program it built and attacks it, as the
security checks of layer L7: requests without a session or without a role, another viewer who
tries to see or change records that are not theirs, markup in every field, and a search of the
app's log and telemetry for what people entered. This guide says how the app runs, who the test
viewers are, what each attack does, what fails a publish and what is only a note, how to let the
attacks reach every page, and what they cannot know.

## Where they run

The attacks are two steps of the delivery pipeline. They come after `tests`, and run only when the
tests passed:

| Step | Layer | What it does |
|---|---|---|
| `attacks` | L7 Simulated attacks | finds the app's pages and forms, as a viewer would, and attacks them |
| `personal-data` | L7 Simulated attacks | searches what the app wrote during the attacks for what was entered in its forms |

One run of the app serves both. `personal-data` is skipped when `attacks` ended in `error`, or
when the app did not start. The app runs on its own, and nothing serves it to people:

- the program that the `build` step made runs under a runner, in a sandbox with no network, in
  the environment `preview`;
- it gets a new, empty database when the permission list declares `sqldb`, and empty file stores;
- its mail is caught and never sent;
- each setting is `l7-` and its name, such as `l7-report_day`, and each secret a random value,
  new for each publish. An `OnStart` that refuses such a value stops the app (E-GATE-036): keep
  the value `OnStart` reads, and check it where the app uses it.

Each request carries a viewer token for one of the test viewers, as a request through the company
login does, or, to test the app without a session, no token or one that is not valid.

## The test viewers

The attacks act as three viewers, none of them in a group:

- **The first viewer**, `l7-a`, holds every role that the app's access rules, and the rules of
  the functions it serves to other apps, name. It finds the pages and enters the records.
- **The other viewer**, `l7-b`, tests each guarded page, one with `guard="true"` in its
  `<ssr:access>`. It holds only the roles that the access rules ask for up to the page that
  declares the `Guard`: one role of each rule on that path, such as `staff`. So it does not hold
  a role that the `Guard` lets see every record.
- **The viewer with no role**, `l7-n`, tests every page whose access rules name a role.

In the room-maintenance example, `/tickets/{id}` declares the `Guard` with `role="staff"`, and the
`Guard` lets in the ticket's author and `hotel-ops`. So the other viewer holds `staff` alone, and
must neither see nor change the tickets the first viewer reported. `/tickets/{id}/edit` asks for
the same roles, and is tested the same way. `/tickets/{id}/status` asks for `hotel-ops` as well,
more than the page that declares the `Guard`, so it is closed by role and not tested for
ownership: the other viewer, without `hotel-ops`, gets 403 there whoever owns the ticket. A page
with `shared="true"` shows every record to everyone its rules admit, so it is not tested for
ownership either.

## How the attacks find pages

The first viewer finds the pages as a person would:

1. It starts at `/` and at every page without an id in its URL. It follows, breadth-first, the
   links, the addresses of forms and the redirects that stay in the app, up to six steps from
   where it started.
2. It posts each form it finds on a page without an id, so the forms that create records run
   first. A form counts as used when it answers with a redirect, as it does when `Process`
   returns `web.Redirect`; the attacks follow the redirect too.
3. After the other viewer's tests, it posts the forms of the pages with an id, so a form that
   cancels or deletes a record runs last.

It fills every field with a value that the field's checks accept:

- a text field or a text area gets a value of its own, such as `l7q4m2x8k1z0`, and an email
  field that value at `example.com`; a field with a `pattern` gets a value that matches it;
- the number fields of a form get one number, the largest `min` among them or else 1, within
  every `max`, and its date fields one date, today or later;
- a list gets its first option with a value, a group of check boxes its first box, and radio
  buttons their first value;
- a file field gets a small file: an image when `accept` names images, or else a text file;
- no value is longer than the field's `maxlength`.

A guarded page with a text parameter that the first viewer did not reach is also tried with that
viewer's own id, as in `/people/l7-a/certificates`, since such a page often shows the viewer's own
records.

The attacks stop finding pages after 4000 requests or 3 minutes, and visit at most 20 URLs of each
page; the whole step ends within 8 minutes, and an answer is read up to 2 MiB. Reaching a limit is
no problem in itself, but each page not reached by then is a note (E-GATE-038).

## What each attack does

A page that the first viewer reached is attacked at the URLs it reached. Any other page is
attacked at a URL made from its path, with `1` for a number in it and `l7` for text, such as
`/trips/1`.

- **No session.** Every page, every form post, every live connection of a page and, when the app
  serves functions to other apps, every call to them is sent with no viewer token, with an
  expired token, and with a token made for another app. Each must answer 401, or the page fails
  with E-GATE-029.
- **No role.** The viewer with no role loads every page whose access rules name a role, posts its
  forms and opens its live connection. Each must answer 403, or the page fails with E-GATE-030. A
  page whose only rule is `role="*"` is open to every viewer the company login lets in, and is
  not tested.
- **Another viewer.** On each guarded page, the other viewer opens every URL of the page that the
  first viewer reached: the page must not show what the first viewer entered, or it fails with
  E-GATE-031. Then it posts each form that the first viewer saw at that URL, with the first
  viewer's values and a form token of its own: the page must refuse it with 403 or 404, or it
  fails with E-GATE-032. A guarded page of which the first viewer reached no URL fails with
  E-GATE-033, since nothing checked its `Guard`. The `Guard` is where a page decides who sees
  each record; see [access](access.md).
- **Markup in fields.** Each form the first viewer used is posted again four times, with markup in
  every text field: an element, an attribute, a link that runs a script, and a script. Then the
  first viewer loads every page it reached again, and reads it as a browser does. A page that
  serves any of them as markup, not as text, fails with E-GATE-034.
- **Personal data in the log.** The `personal-data` step searches, without regard to case,
  everything the app wrote while the attacks ran, its standard output and standard error, and
  every span it recorded, with its name, attributes, events and status, for every value the test
  viewers entered. Each value it finds fails with E-GATE-035. The step reads at most 32 MiB of
  output and 100,000 spans; an app that writes more, or drops spans, fails with E-GATE-037, since
  what the step could not read might hold such a value. See [telemetry](telemetry.md).

## What fails and what is a note

These problems stop the publish, as those of every other step do:

- E-GATE-029 to E-GATE-034, from `attacks`: the attacks above;
- E-GATE-036, from `attacks`: the app did not start, or stopped, while the attacks ran;
- E-GATE-035 and E-GATE-037, from `personal-data`: what someone entered in the app's log or
  telemetry, and more log or telemetry than the step reads;
- E-GATE-012: the app ran out of memory, or a step out of time.

E-GATE-038 is a note, which does not stop the publish: a page the attacks did not reach. The
change record counts the pages, those the attacks reached, and lists those they did not, so
security sees which pages they did not check.

A problem about a page is at its template, at line 1, such as `pages/trips/n_id/index.html:1`;
one about a value entered is at the template of the page with the form; E-GATE-036 and
E-GATE-037 are at `main.go:1`. The message names the request, the roles of the test viewer, what
the app answered and what was expected, and never holds a token:

```text
  trips: pages/trips/n_id/index.html:1: E-GATE-032: POST /trips/7 (form withdraw) as another viewer with the roles staff: answered 303, expected 403 or 404
    fix: check ownership in the page's Guard, which runs before every form, not in Data or Process
    docs: https://aicoded.dev/docs/errors/E-GATE-032
```

When the attacks cannot run through a fault of the delivery pipeline, such as a runner that does
not answer, the step ends in `error`, never in `passed`: publish a new commit later, as after any
`error`.

## Letting the attacks reach every page

The attacks check only what they reach, and they reach a page the way a viewer does:

- **Link every page from a page its viewers reach,** such as a menu, a list of records or a
  button on a record. A page that only a mail, a script or a URL typed by hand leads to is not
  reached.
- **Let a form on a page without an id create the records,** and redirect to the new record or
  to a list that links each one. The database is empty when the attacks start, so a page with an
  id is reached only through a record that one of the app's forms created.
- **Keep a form's checks to what the data needs.** A form that refuses the values the attacks
  fill in creates nothing, and the pages behind it are not reached.

In the room-maintenance example, the menu links `/tickets/report`, whose form reports a ticket and
redirects to it; `/tickets` links each ticket, and a ticket's page links its edit and status
pages. So the attacks reach every page, and test the `Guard` of `/tickets/{id}` and of its edit
page.

## What the attacks cannot know

The attacks test that the app keeps to what its permission list says, not that the list says the
right thing:

- **A page that the app's own permission list opens to everyone** is no finding: a page with
  `role="*"`, or `shared="true"` on a page whose records only some viewers should see, does what
  its permission list says. Security sees the permission list, and what changed in it since the
  last publish, in the change record, so security, not the attacks, decides whether the page
  should be open.
- **A page that is only too strict,** and refuses viewers it should let in, passes: it is safe,
  though wrong. Try each page with the personas of `aicoded dev`.
- **What scripts do in the browser:** the attacks read each page as the app sends it, and do not
  run its scripts.
- **Functions the app serves to other apps** are tested only without a session. The runner and
  the app check their rules on every call; see [calls between apps](calls-between-apps.md).
- **The path of a request** is not searched for what people entered, since the framework records
  it in the request's span: keep values people enter out of URLs.

## The codes

- E-GATE-029 to E-GATE-034 and E-GATE-036: the `attacks` step.
- E-GATE-035 and E-GATE-037: the `personal-data` step.
- E-GATE-038: a page the attacks did not reach, a note.

`aicoded explain <code>` prints the page of each, as does the `howto` tool of `aicoded mcp`.

## See also

- [Publishing](publishing.md): every step of the delivery pipeline, and reading a publish's
  outcome.
- [Access](access.md): access rules, Guards, and whose records a page shows.
- [Check who owns a record](../tasks/ownership-check.md): a `Guard` that the attacks test.
- [Telemetry](telemetry.md): logs and spans with no personal data.
- [The error catalogue](../errors/README.md): every code with its page.
