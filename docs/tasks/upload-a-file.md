# Take a file upload

Take files in a form, check them, and keep them in a file store. The example is the photos of a
new user in the people example, which it keeps in its store `photos` as `<login>/<file name>`.

## Steps

1. Declare the store in the permission list, under `data`: `- source: filestore:photos` with
   `classes: [internal]`. Open it once, in `Deps.Start`, with `filestore.Open(ctx, "photos")`,
   as [Start a new app](new-app.md) shows.
2. Add file fields to the form, such as
   `<ssr:input name="photo" type="file" required accept="image/*" id="photo"/>`, with `multiple`
   for several files. A form with a file field posts as `multipart/form-data`; people's says so
   with `enctype="multipart/form-data"`. In `Process`, a field's value is a `*form.FileHeader`,
   or a slice of them with `multiple`.
3. Check every file before you keep it: its name, its size and its content. The browser's
   `accept` and the file's name prove nothing about what it holds.

   <!-- code: examples/people/pages/users/add/dataprovider.go maxPhoto -->
   ```go
   // maxPhoto is the largest photo the form takes, in bytes.
   const maxPhoto = 5 << 20
   ```

   <!-- code: examples/people/pages/users/add/dataprovider.go fileNamePattern -->
   ```go
   var fileNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,99}$`)
   ```

   <!-- code: examples/people/pages/users/add/dataprovider.go readPhoto -->
   ```go
   // readPhoto reads an uploaded photo. It returns what is wrong with the file, or "".
   func readPhoto(fh *form.FileHeader) (deps.Photo, string) {
   	if !fileNamePattern.MatchString(fh.Filename) {
   		return deps.Photo{}, "Rename the file: use letters, digits, spaces, dots, dashes and underscores."
   	}
   	if fh.Size > maxPhoto {
   		return deps.Photo{}, "Choose a photo of at most 5 MiB."
   	}
   	file, err := fh.Open()
   	if err != nil {
   		return deps.Photo{}, "The photo could not be read."
   	}
   	defer file.Close()
   	data, err := io.ReadAll(io.LimitReader(file, maxPhoto+1))
   	switch {
   	case err != nil:
   		return deps.Photo{}, "The photo could not be read."
   	case len(data) > maxPhoto:
   		return deps.Photo{}, "Choose a photo of at most 5 MiB."
   	case !strings.HasPrefix(http.DetectContentType(data), "image/"):
   		return deps.Photo{}, "Choose an image file."
   	}
   	return deps.Photo{Name: fh.Filename, Data: data}, ""
   }
   ```

   <!-- code: examples/people/pages/users/add/dataprovider.go readPhotos -->
   ```go
   // readPhotos returns the uploaded photos, and sets an error on each field whose files are wrong.
   func readPhotos(f *FormAddValues) []deps.Photo {
   	photo, problem := readPhoto(f.Photo.GetValue())
   	if problem != "" {
   		f.Photo.SetError(problem)
   		return nil
   	}
   	photos := []deps.Photo{photo}
   	if len(f.Photos.GetValue()) > 4 {
   		f.Photos.SetError("Choose at most 4 more photos.")
   		return nil
   	}
   	for _, fh := range f.Photos.GetValue() {
   		p, problem := readPhoto(fh)
   		if problem == "" && slices.ContainsFunc(photos, func(q deps.Photo) bool { return q.Name == p.Name }) {
   			problem = "Give each photo its own file name."
   		}
   		if problem != "" {
   			f.Photos.SetError(problem)
   			return nil
   		}
   		photos = append(photos, p)
   	}
   	return photos
   }
   ```

   A request is at most 32 MiB in all; a larger one is refused with 413.
4. Write the files while the record's transaction is open, and commit only when every file is
   stored. A file appears, whole, only once its write succeeds, and its folder must exist first.

   <!-- code: examples/people/deps/users.go Deps.AddUser -->
   ```go
   // AddUser adds u and stores its photos as <login>/<file name> in one transaction: the user is
   // added only when every photo is stored, and an add that fails removes the photos it stored and
   // the folder it made. It returns ErrLoginTaken when another user has the login.
   func (d *Deps) AddUser(ctx context.Context, u User, photos []Photo) error {
   	return addUser(ctx, d.DB, d.Photos, u, photos)
   }
   ```

   It passes the app's database and photo store to `addUser`, which takes the store as a
   `photoStore`, the part of `*filestore.Store` it uses, so that a test can pass a store in
   memory:

   <!-- code: examples/people/deps/users.go photoStore -->
   ```go
   // photoStore is what addUser uses of a *filestore.Store, so that a test can pass a store in
   // memory.
   type photoStore interface {
   	Stat(name string) (fs.FileInfo, error)
   	Mkdir(name string) error
   	WriteFile(name string, data []byte) error
   	Remove(name string) error
   }
   ```

   <!-- code: examples/people/deps/users.go addUser -->
   ```go
   // addUser is AddUser with the database db and the photo store store.
   func addUser(ctx context.Context, db *sql.DB, store photoStore, u User, photos []Photo) (err error) {
   	ctx, span := telemetry.Start(ctx, "users.add")
   	span.SetAttr("photos", strconv.Itoa(len(photos)))
   	defer func() {
   		if err != nil {
   			span.RecordError(err)
   		}
   		span.End()
   	}()
   	tx, err := db.BeginTx(ctx, nil)
   	if err != nil {
   		return err
   	}
   	defer func() { _ = tx.Rollback() }()
   	var taken bool
   	if err := tx.QueryRowContext(ctx, loginExists, u.Login).Scan(&taken); err != nil {
   		return err
   	}
   	if taken {
   		return ErrLoginTaken
   	}
   	if err := insert(ctx, tx, u); err != nil {
   		// An add of the same login that committed after the check above makes the UNIQUE index
   		// refuse this one, with an error that names the login: check again, outside this
   		// transaction.
   		if db.QueryRowContext(ctx, loginExists, u.Login).Scan(&taken) == nil && taken {
   			return ErrLoginTaken
   		}
   		return err
   	}
   	var stored []string // the folder this add made and the files it wrote, in that order
   	defer func() {
   		if err != nil {
   			if rerr := removePhotos(store, stored); rerr != nil {
   				err = errors.Join(err, rerr)
   			}
   		}
   	}()
   	if len(photos) > 0 {
   		_, serr := store.Stat(u.Login)
   		if err := store.Mkdir(u.Login); err != nil {
   			return err
   		}
   		if errors.Is(serr, fs.ErrNotExist) {
   			stored = append(stored, u.Login)
   		}
   	}
   	for _, p := range photos {
   		name := u.Login + "/" + p.Name
   		if err := store.WriteFile(name, p.Data); err != nil {
   			return err
   		}
   		stored = append(stored, name)
   	}
   	return tx.Commit()
   }
   ```

   `loginExists` is `SELECT EXISTS (SELECT 1 FROM users WHERE login = ?)`. Two adds of one login
   at once can both pass that check; the UNIQUE index on `login` then refuses the second insert
   with an error that names the login, so `addUser` checks again and returns `ErrLoginTaken`.

   A photo stored by an add that then fails would stay with no user, and a later user of the same
   login would see it. So a failed add removes what it stored, last first, and the folder only
   when it made it. An error of that removal joins the add's error and never replaces it:

   <!-- code: examples/people/deps/users.go removePhotos -->
   ```go
   // removePhotos removes names from store, last first, and returns the first error.
   func removePhotos(store photoStore, names []string) error {
   	var first error
   	for _, name := range slices.Backward(names) {
   		if err := store.Remove(name); err != nil && first == nil {
   			first = err
   		}
   	}
   	return first
   }
   ```

   An error from the store names the file, in its path and often in the runner's message, and the
   name holds the login and the uploaded file's name. `addUser` returns it as it is: outside
   `aicoded dev`, the framework logs and traces it only by its kinds and codes, such as
   `fs.PathError write: fs.ErrNotExist`, and `aicoded check` refuses app code that logs it
   (E-LINT-008).

5. Read the files back. A store is an `fs.FS`, with `ReadDir`, `Stat` and `ReadFile`:

   <!-- code: examples/people/deps/users.go Deps.PhotosOf -->
   ```go
   // PhotosOf returns the photos of login, sorted by file name.
   func (d *Deps) PhotosOf(login string) ([]PhotoFile, error) {
   	entries, err := d.Photos.ReadDir(login)
   	if errors.Is(err, fs.ErrNotExist) {
   		return nil, nil
   	}
   	if err != nil {
   		return nil, err
   	}
   	files := make([]PhotoFile, 0, len(entries))
   	for _, e := range entries {
   		info, err := e.Info()
   		if err != nil {
   			return nil, err
   		}
   		files = append(files, PhotoFile{Name: info.Name(), Size: info.Size()})
   	}
   	return files, nil
   }
   ```

   A page cannot send a stored file to the browser yet; that comes later. The user's card shows
   each photo's name and size:

   <!-- code: examples/people/pages/users/s_login/dataprovider.go DP.Data -->
   ```go
   // Data shows the user's card, with the name and size of each stored photo. A login no user has
   // is 404.
   func (p *DP) Data(ctx context.Context, r *web.Request, _ web.ResponseWriter, data *RouteData) error {
   	u, err := p.d.User(ctx, r.URLParam("login"))
   	if errors.Is(err, deps.ErrNoUser) {
   		return web.NotFound()
   	}
   	if err != nil {
   		return err
   	}
   	photos, err := p.d.PhotosOf(u.Login)
   	if err != nil {
   		return err
   	}
   	data.User, data.Initials, data.Photos = u, initials(u.Name), photos
   	data.TabClass = func(tab string) string {
   		if path.Base(r.URL.Path) == tab {
   			return "active"
   		}
   		return ""
   	}
   	return nil
   }
   ```

## Check it

- `aicoded check` generates, builds, vets, lints and tests the app.
- Add a user with a photo at `http://people.localhost:8080/users/add`, as a persona with the
  roles `staff` and `hr`. The user's card lists `Photo: <name>, <size> bytes`. A file that is not
  an image gets `Choose an image file.`
- `aicoded dev` keeps the files in its private state folder, under `files/people/photos/`.

## See also

- [File stores](../guides/file-stores.md)
- [Forms](../guides/forms.md)
