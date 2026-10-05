package add

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"aicoded.dev/framework/mailer"
	"aicoded.dev/framework/web"
	"aicoded.dev/framework/web/form"

	"people/deps"
)

var _ RouteDataProvider = &DP{}

// DP provides the form that adds a user.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Data needs nothing: the page is its form.
func (p *DP) Data(context.Context, *web.Request, web.ResponseWriter, *RouteData) error { return nil }

// InitAdd offers the teams and languages, and starts with a sample name and the day shift.
func (p *DP) InitAdd(_ context.Context, _ *web.Request, _ web.ResponseWriter, f *FormAddValues) error {
	f.Name.SetValue("John Doe")
	f.Shift.SetValue(1)
	f.Team.SetOptions([]form.SelectOptionElement[string]{
		form.SelectOptionGroup[string]{Label: "Hotel", Options: []form.SelectOptionElement[string]{
			option("Front desk"),
			option("Housekeeping"),
			form.SelectOption[string]{Value: "Spa", Label: "Spa (closed)", Disabled: true},
		}},
		form.SelectOptionGroup[string]{Label: "Head office", Options: []form.SelectOptionElement[string]{
			option("Finance"), option("Marketing"), option("People"), option("IT"),
		}},
	})
	f.Languages.SetOptions([]form.SelectOptionElement[string]{
		option("English"), option("French"), option("German"), option("Portuguese"), option("Spanish"),
	})
	return nil
}

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

var loginPattern = regexp.MustCompile(`^[a-z0-9]{1,32}$`)

// readUser returns the user the form describes, and sets an error on each field that is wrong.
func readUser(f *FormAddValues) deps.User {
	u := deps.User{
		Login:     f.Login.GetValue(),
		Name:      strings.TrimSpace(f.Name.GetValue()),
		Age:       int(f.Age.GetValue()),
		Team:      f.Team.GetValue(),
		Languages: strings.Join(slices.Sorted(maps.Keys(f.Languages.GetValue())), ", "),
		Shift:     int(f.Shift.GetValue()),
		Bio:       strings.TrimSpace(f.Bio.GetValue()),
	}
	_, u.OnSite = f.Remote.GetValue()[false]
	_, u.Remote = f.Remote.GetValue()[true]
	switch {
	case !loginPattern.MatchString(u.Login):
		f.Login.SetError("Use 1 to 32 lowercase letters and digits.")
	case u.Login == "add":
		f.Login.SetError("That login is taken.")
	}
	if u.Name == "" {
		f.Name.SetError("Enter a name.")
	} else if utf8.RuneCountInString(u.Name) > 100 {
		f.Name.SetError("Use at most 100 characters.")
	}
	if u.Age < 18 || u.Age > 120 {
		f.Age.SetError("Enter an age from 18 to 120.")
	}
	if !u.OnSite && !u.Remote {
		f.Remote.SetError("Choose at least one.")
	}
	if u.Shift != 1 && u.Shift != 2 {
		f.Shift.SetError("Choose a shift.")
	}
	if u.Bio == "" {
		f.Bio.SetError("Write a few words about them.")
	} else if utf8.RuneCountInString(u.Bio) > 2000 {
		f.Bio.SetError("Use at most 2000 characters.")
	}
	return u
}

// maxPhoto is the largest photo the form takes, in bytes.
const maxPhoto = 5 << 20

var fileNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,99}$`)

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

func option(name string) form.SelectOption[string] {
	return form.SelectOption[string]{Value: name, Label: name}
}
