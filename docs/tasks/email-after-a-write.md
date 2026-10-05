# Send mail after a write

Mail someone once a change is saved, and only once: commit the change first, then send the mail
with an idempotency key that names what it is about. The example adds a user in the people
example and mails the team.

## Steps

1. Declare mail in the permission list: `email.from`, the one address the app sends from, and
   `email.to_domains`, the only domains it may send to. people has `from: people@acme.example`
   and `to_domains: [acme.example]`, and reads the team's address from the setting
   `team_address`.
2. Make the change in one transaction, and commit it before anything leaves the app:

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

   `Deps.AddUser` calls it with the app's database and photo store. Its span, `users.add`,
   records only how many photos there are: no login, name or file name reaches telemetry.
3. Send the mail once the change is committed, with `IdempotencyKey`:

   <!-- code: examples/people/pages/users/add/dataprovider.go DP.ProcessAdd -->
   ```go
   // ProcessAdd adds the user with their photos, tells the open users pages and the team, and
   // opens the new user's page. It logs nothing about the form.
   func (p *DP) ProcessAdd(ctx context.Context, _ *web.Request, _ web.ResponseWriter, f *FormAddValues) error {
   	u := readUser(f)
   	photos := readPhotos(f)
   	if f.HasError() {
   		return nil
   	}
   	err := p.d.AddUser(ctx, u, photos)
   	if errors.Is(err, deps.ErrLoginTaken) {
   		f.Login.SetError("That login is taken.")
   		return nil
   	}
   	if err != nil {
   		return err
   	}
   	p.d.UsersAdded.Publish(struct{}{})
   	_, err = mailer.Send(ctx, mailer.Message{
   		IdempotencyKey: "user-" + u.Login,
   		To:             []mailer.Address{{Address: p.d.TeamAddress}},
   		Subject:        "User " + u.Login + " added",
   		Text:           fmt.Sprintf("%s (%s) joined the team %s.\n", u.Name, u.Login, u.Team),
   	})
   	if err != nil {
   		return err
   	}
   	return web.Redirect("/users/" + u.Login)
   }
   ```

   - The key names what the mail is about, `user-<login>`. Sending the same key again sends
     nothing and returns the first message's id, so a retried request never mails twice. A
     message without a key is refused (E-MAIL-001).
   - A transaction that fails sends nothing, since the mail goes only after the commit.
   - The sender is always `email.from`, and the domain of every recipient must be one of
     `to_domains` exactly (E-MAIL-003).
4. When the mail is about what the viewer wrote rather than about a record, make the key from
   what was written. The contact form hashes the viewer, the topic and the message, so a message
   sent twice is mailed once:

   <!-- code: examples/people/pages/contact/dataprovider.go contactKey -->
   ```go
   // contactKey names a message by its sender, topic and text, so that a message sent twice is
   // mailed once.
   func contactKey(subject, topic, message string) string {
   	sum := sha256.Sum256([]byte(subject + "\x00" + topic + "\x00" + message))
   	return "contact-" + hex.EncodeToString(sum[:16])
   }
   ```

   [Add a form](add-form.md) shows the whole form.

Put no form value in a log line or a span, which `aicoded check` refuses (E-LINT-008): the mail
carries it to the team, and nothing else needs it.

## Check it

- `aicoded check` generates, builds, vets, lints and tests the app.
- `aicoded dev` catches mail instead of sending it, after checking it against the mail rules and
  the permission list. Add a user as a persona with the roles `staff` and `hr`, and the dev UI's
  Mail page shows one message, `User <login> added`.
- The dev UI's Traces page shows the request with the spans `users.add` and `mailer.send`.

## See also

- [Mail](../guides/mail.md)
- [SQL database](../guides/sql-database.md)
- [Telemetry](../guides/telemetry.md)
