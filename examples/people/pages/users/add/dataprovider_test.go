package add

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// upload is a file posted in the field photo or photos. A size other than 0 replaces the size
// the request gave for it.
type upload struct {
	field, name string
	data        []byte
	size        int64
}

func TestAddRefuses(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n")
	photo := upload{"photo", "a.png", png, 0}
	more := func(name string) upload { return upload{"photos", name, png, 0} }
	for i, c := range []struct {
		login          string
		uploads        []upload
		field, problem string
	}{
		{"alice2", []upload{photo, more("b.png"), more("c.png"), more("d.png"), more("e.png")}, "", ""},
		{"al/ce", []upload{photo}, "login", "Use 1 to 32 lowercase letters and digits."},
		{strings.Repeat("a", 33), []upload{photo}, "login", "Use 1 to 32 lowercase letters and digits."},
		{"add", []upload{photo}, "login", "That login is taken."},
		{"", []upload{photo}, "login", "Use 1 to 32 lowercase letters and digits."},
		{"Alice", []upload{photo}, "login", "Use 1 to 32 lowercase letters and digits."},
		{"alice ", []upload{photo}, "login", "Use 1 to 32 lowercase letters and digits."},
		{"alice", []upload{{"photo", ".a.png", png, 0}}, "photo", "Rename the file: use letters, digits, spaces, dots, dashes and underscores."},
		{"alice", []upload{{"photo", "a.png", []byte("plain text"), 0}}, "photo", "Choose an image file."},
		{"alice", []upload{{"photo", "a.png", png, maxPhoto + 1}}, "photo", "Choose a photo of at most 5 MiB."},
		{"alice", []upload{{"photo", "a.png", append(png, make([]byte, maxPhoto)...), 1}}, "photo", "Choose a photo of at most 5 MiB."},
		{"alice", []upload{photo, more("b.png"), more("a.png")}, "photos", "Give each photo its own file name."},
		{"alice", []upload{photo, more("b.png"), more("c.png"), more("d.png"), more("e.png"), more("f.png")}, "photos", "Choose at most 4 more photos."},
	} {
		f := post(t, c.login, c.uploads)
		u, photos := readUser(f), readPhotos(f)
		want := map[string]string{"login": "", "photo": "", "photos": ""}
		if c.field != "" {
			want[c.field] = c.problem
		} else {
			assert.Equal(t, c.login, u.Login, "case %d", i)
			assert.Len(t, photos, len(c.uploads), "case %d", i)
		}
		got := map[string]string{"login": f.Login.GetError(), "photo": f.Photo.GetError(), "photos": f.Photos.GetError()}
		assert.Equal(t, want, got, "case %d", i)
	}
}

// post returns the add form posted with login and uploads, parsed as the generated code does.
func post(t *testing.T, login string, uploads []upload) *FormAddValues {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for _, u := range uploads {
		part, err := w.CreateFormFile(u.field, u.name)
		require.NoError(t, err)
		_, err = part.Write(u.data)
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	mf, err := multipart.NewReader(&body, w.Boundary()).ReadForm(64 << 20)
	require.NoError(t, err)
	t.Cleanup(func() { _ = mf.RemoveAll() })
	for _, u := range uploads {
		if u.size != 0 {
			for _, fh := range mf.File[u.field] {
				fh.Size = u.size
			}
		}
	}
	r := &http.Request{MultipartForm: mf}
	f := &FormAddValues{}
	f.Login.SetValue(login)
	f.Photo.Process(r, "photo", true, true)
	f.Photos.Process(r, "photos", true, false)
	return f
}
