// Package sources records each kind of outside data. A line that records it ends in a comment
// "leak:" with the finding it makes; a line that records what may be recorded ends in "clean".
package sources

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"strconv"
	"strings"

	"aicoded.dev/framework/auth"
	"aicoded.dev/framework/filestore"
	"aicoded.dev/framework/mailer"
	"aicoded.dev/framework/telemetry"
	"aicoded.dev/framework/web"
	"aicoded.dev/framework/web/form"

	"taint/services/billing"
)

// Form has a field of each kind.
type Form struct {
	Login  form.Input[string]
	Age    form.Input[int]
	Langs  form.InputMultiple[string]
	Team   form.Select[string]
	Tags   form.SelectMultiple[string]
	Bio    form.Textarea
	Photo  form.File
	Photos form.FileMultiple
}

// FormValues records the values of f and of the files uploaded with it.
func FormValues(ctx context.Context, f *Form) {
	slog.InfoContext(ctx, "login", "login", f.Login.GetValue())  // leak: a form value reaches slog.InfoContext
	slog.InfoContext(ctx, "typed", "text", f.Age.GetFormValue()) // leak: a form value reaches slog.InfoContext
	slog.InfoContext(ctx, "age", "age", f.Age.GetValue())        // clean
	for lang := range f.Langs.GetValue() {
		fmt.Println(lang) // leak: a form value reaches fmt.Println
	}
	fmt.Printf("team %s\n", f.Team.GetValue())                   // leak: a form value reaches fmt.Printf
	slog.Info("tags", "tags", f.Tags.GetValue())                 // leak: a form value reaches slog.Info
	slog.Info("tags", "n", len(f.Tags.GetValue()))               // clean
	print(f.Bio.GetValue())                                      // leak: a form value reaches print
	slog.Info("photo", "name", f.Photo.GetValue().Filename)      // leak: a form value reaches slog.Info
	slog.Info("photo", "size", f.Photo.GetValue().Size)          // clean
	slog.Info("photos", "name", f.Photos.GetValue()[0].Filename) // leak: a form value reaches slog.Info
	file, err := f.Photo.GetValue().Open()
	if err != nil {
		return
	}
	defer file.Close()
	data, _ := io.ReadAll(file)
	slog.Info("photo", "bytes", data) // leak: a form value reaches slog.Info
}

// Request records parts of the request.
func Request(r *web.Request) {
	slog.Info("agent", "agent", r.Header.Get("User-Agent")) // leak: the request reaches slog.Info
	slog.Info("path", "path", r.URL.Path)                   // leak: the request reaches slog.Info
	if c, err := r.Cookie("theme"); err == nil {
		fmt.Println(c.Value) // leak: the request reaches fmt.Println
	}
	body, _ := io.ReadAll(r.Body)
	println(string(body))                            // leak: the request reaches println
	slog.Info("login", "login", r.URLParam("login")) // leak: a URL parameter reaches slog.Info
	slog.Info("id", "id", r.URLParamInt("id"))       // clean
	slog.Info("post", "post", r.Method == "POST")    // clean
	slog.InfoContext(r.Context(), "served")          // clean
}

// Viewer records the viewer.
func Viewer(ctx context.Context) {
	v := auth.Viewer(ctx)
	slog.InfoContext(ctx, "viewer", "subject", v.Subject)         // leak: the viewer's identity reaches slog.InfoContext
	_, span := telemetry.Start(ctx, "greet "+v.Name)              // leak: the viewer's identity reaches telemetry.Start
	span.SetAttr("groups", strings.Join(v.Groups, ","))           // leak: the viewer's identity reaches telemetry.Span.SetAttr
	span.SetAttr("staff", strconv.FormatBool(v.HasRole("staff"))) // clean
	span.End()
}

// user is a row of the users table.
type user struct {
	Login string
	Age   int
}

// Database records values read from the database.
func Database(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, "SELECT name FROM users")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			slog.ErrorContext(ctx, "scan", "err", err) // leak: a value from the database reaches slog.ErrorContext
			return err
		}
		slog.InfoContext(ctx, "user", "name", name) // leak: a value from the database reaches slog.InfoContext
	}
	var n int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&n); err != nil {
		return err
	}
	slog.InfoContext(ctx, "users", "n", n) // clean
	u, err := scan(db.QueryRowContext(ctx, "SELECT login, age FROM users LIMIT 1"))
	if err != nil {
		return err
	}
	slog.InfoContext(ctx, "first", "login", u.Login) // leak: a value from the database reaches slog.InfoContext
	slog.InfoContext(ctx, "first", "age", u.Age)     // clean
	return rows.Err()
}

// scan reads a user from a row.
func scan(row interface{ Scan(...any) error }) (user, error) {
	var u user
	err := row.Scan(&u.Login, &u.Age)
	return u, err
}

// Name is a column that reads itself.
type Name string

// Scan reads src, a value from the database.
func (n *Name) Scan(src any) error {
	slog.Info("scanned", "src", src) // leak: a value from the database reaches slog.Info
	s, _ := src.(string)
	*n = Name(s)
	return nil
}

// Inbox records mails from the inbox.
func Inbox(ctx context.Context) error {
	list, _, err := mailer.List(ctx, mailer.Inbox, 0, 10)
	if err != nil {
		return err
	}
	for _, s := range list {
		slog.InfoContext(ctx, "mail", "subject", s.Subject) // leak: a mail from the inbox reaches slog.InfoContext
		slog.InfoContext(ctx, "mail", "size", s.Size)       // clean
	}
	m, err := mailer.Get(ctx, "m1")
	if err != nil {
		return err
	}
	fmt.Print(m.Text) // leak: a mail from the inbox reaches fmt.Print
	return nil
}

// Files records stored files: their names, contents and errors.
func Files(ctx context.Context, store *filestore.Store) error {
	data, err := store.ReadFile("notes/a.txt")
	if err != nil {
		slog.ErrorContext(ctx, "read", "err", err) // leak: a stored file reaches slog.ErrorContext
		return err
	}
	slog.InfoContext(ctx, "note", "text", string(data)) // leak: a stored file reaches slog.InfoContext
	entries, err := store.ReadDir("notes")
	if err != nil {
		return err
	}
	for _, e := range entries {
		fmt.Println(e.Name()) // leak: a stored file reaches fmt.Println
	}
	info, err := fs.Stat(store, "notes/a.txt")
	if err != nil {
		return err
	}
	slog.Info("name", "name", info.Name()) // leak: a stored file reaches slog.Info
	slog.Info("size", "size", info.Size()) // clean
	if err := store.WriteFile("notes/b.txt", nil); err != nil {
		var pe *fs.PathError
		if errors.As(err, &pe) {
			slog.Info("path", "path", pe.Path) // leak: a stored file reaches slog.Info
		}
	}
	return fs.WalkDir(store, ".", func(path string, _ fs.DirEntry, err error) error {
		slog.Info("file", "path", path) // leak: a stored file reaches slog.Info
		return err
	})
}

// DP provides a page with a page call and a live value.
type DP struct{}

// StarIn is the input of the page call star.
type StarIn struct {
	Note    string
	Starred bool
}

// CallStar stars a note.
func (p *DP) CallStar(ctx context.Context, _ *web.Request, in StarIn) (bool, error) {
	slog.InfoContext(ctx, "star", "note", in.Note)       // leak: the input of a page call reaches slog.InfoContext
	slog.InfoContext(ctx, "star", "starred", in.Starred) // clean
	return in.Starred, nil
}

// ValidateName accepts any name.
func (p *DP) ValidateName(_ context.Context, _ *web.Request, val string) (string, error) {
	fmt.Println(val) // leak: a live value from the browser reaches fmt.Println
	return val, nil
}

// Billing records what another app answers.
func Billing(ctx context.Context) error {
	inv, err := billing.GetInvoice(ctx, billing.InvoiceID{ID: 7})
	if err != nil {
		slog.ErrorContext(ctx, "invoice", "err", err) // leak: the answer of another app reaches slog.ErrorContext
		return err
	}
	slog.InfoContext(ctx, "invoice", "customer", inv.Customer) // leak: the answer of another app reaches slog.InfoContext
	slog.InfoContext(ctx, "invoice", "id", inv.ID)             // clean
	return nil
}

// SignUp is a form with a form-wide error.
type SignUp struct {
	form.BaseFormValues
	Login form.Input[string]
	Team  form.Select[string]
}

// FieldErrors records what a page set on its forms: errors that quote the URL parameter login, a
// value, and options read from the database.
func FieldErrors(ctx context.Context, r *web.Request, db *sql.DB) error {
	login := r.URLParam("login")
	var f SignUp
	f.Login.SetError("'" + login + "' is taken")
	slog.WarnContext(ctx, "field", "err", f.Login.GetError()) // leak: a URL parameter reaches slog.WarnContext
	var g SignUp
	g.SetError("no team for " + login)
	slog.WarnContext(ctx, "form", "err", g.GetError()) // leak: a URL parameter reaches slog.WarnContext
	var bio form.Textarea
	bio.SetValue(login)
	slog.InfoContext(ctx, "bio", "field", bio) // leak: a URL parameter reaches slog.InfoContext
	var team string
	if err := db.QueryRowContext(ctx, "SELECT name FROM teams LIMIT 1").Scan(&team); err != nil {
		return err
	}
	var h SignUp
	h.Team.SetOptions([]form.SelectOptionElement[string]{form.SelectOption[string]{Value: team, Label: team}})
	slog.InfoContext(ctx, "options", "n", len(h.Team.GetOptions()))   // clean
	slog.InfoContext(ctx, "options", "first", h.Team.GetOptions()[0]) // leak: a value from the database reaches slog.InfoContext
	return nil
}

// Options records options of a select field that hold the URL parameter login, written as HTML.
func Options(r *web.Request) {
	login := r.URLParam("login")
	none := func(string) bool { return false }
	var b strings.Builder
	_ = form.SelectOption[string]{Value: login, Label: login}.WriteHtml(&b, none)
	slog.Info("option", "html", b.String()) // leak: a URL parameter reaches slog.Info
	var g strings.Builder
	_ = form.SelectOptionGroup[string]{Label: login}.WriteHtml(&g, none)
	slog.Info("group", "html", g.String()) // leak: a URL parameter reaches slog.Info
}
