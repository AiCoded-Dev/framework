package form

import (
	"mime/multipart"
	"net/http"
	"strings"
)

// BaseFormValues is what every form has besides its fields: its id, the viewer's form token,
// a form-wide error and whether the posted values were checked.
type BaseFormValues struct {
	id        string
	token     string
	error     string
	validated bool
	elements  []Element
}

// Element is a form field.
type Element interface {
	HasError() bool
}

// SetElements sets the fields HasError looks at. Generated code calls it.
//
// Generated code only.
func (f *BaseFormValues) SetElements(elements []Element) { f.elements = elements }

// Prepare sets the form's id and the viewer's token, which the rendered form carries in
// hidden inputs. Generated code calls it.
//
// Generated code only.
func (f *BaseFormValues) Prepare(id, token string) { f.id, f.token = id, token }

// ID returns the form's id, as posted in the hidden input web.FormField.
func (f *BaseFormValues) ID() string { return f.id }

// Token returns the token posted in the hidden input web.CSRFField.
func (f *BaseFormValues) Token() string { return f.token }

// MarkValidated records that the posted values were checked. Generated code calls it.
//
// Generated code only.
func (f *BaseFormValues) MarkValidated() { f.validated = true }

// IsValidated reports whether the form was submitted and its values checked.
func (f *BaseFormValues) IsValidated() bool { return f.validated }

// SetError sets an error about the form as a whole.
func (f *BaseFormValues) SetError(err string) { f.error = err }

// GetError returns the error about the form as a whole.
func (f *BaseFormValues) GetError() string { return f.error }

// HasError reports whether the form or any of its fields has an error.
func (f *BaseFormValues) HasError() bool {
	if f.error != "" {
		return true
	}

	for _, e := range f.elements {
		if e.HasError() {
			return true
		}
	}

	return false
}

// IsMultipart reports whether r carries a multipart/form-data body.
//
// Generated code only.
func IsMultipart(r *http.Request) bool {
	return strings.HasPrefix(r.Header.Get("Content-Type"), MultipartFormData)
}

// values returns the values posted for name, or nil when the form was not parsed as
// isMultipart says.
func values(r *http.Request, name string, isMultipart bool) []string {
	if !isMultipart {
		return r.PostForm[name]
	}
	if r.MultipartForm == nil {
		return nil
	}
	return r.MultipartForm.Value[name]
}

// files returns the files posted for name, or nil when the form was not parsed as multipart.
func files(r *http.Request, name string, isMultipart bool) []*multipart.FileHeader {
	if !isMultipart || r.MultipartForm == nil {
		return nil
	}
	return r.MultipartForm.File[name]
}
