# File stores

A file store keeps the app's files, such as uploaded photos or documents. `filestore.Open(ctx,
"photos")` returns the store the permission list declares as `filestore:photos`; the runner keeps
the files, a store holds only this app's files, and no name can lead out of it.

## Declaring and opening a store

```yaml
data:
  - source: filestore:photos
    classes: [internal]
```

Open it once, like the database, in `OnStart`, and share it through `deps.Deps`:

```go
photos, err := filestore.Open(ctx, "photos")
if err != nil {
	return err
}
```

A store the permission list does not declare fails with E-MAN-011. A store's operations keep the
values of the context `Open` got but not its cancellation, so a store opened at start serves
every request.

## Reading

A `*filestore.Store` is an `fs.FS` with `Stat`, `ReadDir`, sorted by name, and `ReadFile`, so
`fs.WalkDir`, `fs.Glob` and `template.ParseFS` work with it.

```go
data, err := p.d.Photos.ReadFile("alice/portrait.png")
entries, err := p.d.Photos.ReadDir("alice")
```

A missing file is an error that `errors.Is(err, fs.ErrNotExist)` matches.

## Writing

- `WriteFile(name, data)` writes a whole file.
- `Create(name)` returns an `io.WriteCloser`. The file appears, whole, only when `Close`
  succeeds, and then replaces a file of the same name; until then the store is unchanged. Always
  call `Close`.
- `Mkdir(name)` makes a folder and any missing parents. The folder of a file must exist before
  the file is written.
- `Remove(name)` removes a file or an empty folder.
- `Rename(old, new)` moves a file or a folder. It replaces a file at the new name, never a
  folder.

`Create` cannot abandon a write: `Close` keeps what was written so far. For data that can fail
half way, such as an upload, read it whole and check it first, then write it with `WriteFile`,
as the people example does with its photos.

## Names

Names are paths inside the store, such as `invoices/2026/42.pdf`: separated by `/`, with no
leading `/`, no empty, `.` or `..` parts, and no part starting with `.aicoded-tmp-`, which the
runner uses for files being written. The store itself, `.`, cannot be written, removed or renamed
(E-FILE-001). A file is at most 1 GiB (E-FILE-002); a write over it stops and leaves the store
unchanged.

A name a viewer chose is input like any other: check it before you use it, as the people example
does with `^[A-Za-z0-9][A-Za-z0-9 ._-]{0,99}$`. An error of a store carries the file's name, in its
path and often in the runner's message too, and a name may hold personal data. Return the error
as it is: outside `aicoded dev`, the framework logs and traces a store's error without either, by
its operation and kind, such as `fs.PathError write: fs.ErrNotExist`. Never log it yourself:
`aicoded check` refuses a store's error, a file's name and its contents in a log line, a print or
a span (E-LINT-008).

## Showing a stored file

A page cannot send a stored file to the browser yet: pages answer only HTML, and a `{{ }}` URL
takes only `http`, `https` and `mailto`, so a `data:` URL made from a file is replaced. Serving
stored files comes later. Until then, show what a page can, such as a file's name and size, as
the user card of the people example does.

## Traces

Every operation is a span named `filestore.<operation>`, such as `filestore.write`, with the
attribute `store`. Outside `aicoded dev`, a failed operation records only its status and error
code, such as `not_found` or `invalid_argument E-FILE-001`, since its message names files. See
[telemetry](telemetry.md).

## In aicoded dev

File stores live in the workspace's private state folder, readable by you only:
`~/.local/state/aicoded/<hash>/files/<app>/<store>`. `aicoded dev` refuses to start when others
can read that folder (E-DEV-009).

## See also

- [Forms](forms.md): file inputs and what to check in an upload.
- [The permission list](permission-list.md): the `data` section.
- [people/deps/users.go](../../examples/people/deps/users.go): photos stored with a new user, and
  listed with their sizes.
- [Take a file upload](../tasks/upload-a-file.md): file fields, their checks and a file store.
