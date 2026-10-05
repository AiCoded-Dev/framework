package form

import (
	"mime/multipart"
	"net/http"
)

// FileHeader describes an uploaded file. Open reads its content.
type FileHeader struct {
	*multipart.FileHeader
}

// File is a file <input> field.
type File struct {
	value *FileHeader
	error string
}

// SetError sets the field's error.
func (e *File) SetError(err string) { e.error = err }

// HasError reports whether the field has an error.
func (e *File) HasError() bool { return e.error != "" }

// GetError returns the field's error.
func (e *File) GetError() string { return e.error }

// GetValue returns the uploaded file, or nil.
func (e *File) GetValue() *FileHeader { return e.value }

// IsNotNull reports whether a file was uploaded.
func (e *File) IsNotNull() bool { return e.value != nil }

// Process reads the field from the parsed form in r. Generated code calls it.
//
// Generated code only.
func (e *File) Process(r *http.Request, name string, isMultipart, isRequired bool) {
	if fs := files(r, name, isMultipart); len(fs) > 0 {
		e.value = &FileHeader{fs[0]}
	}

	if isRequired && e.value == nil {
		e.error = MessageRequiredField
		return
	}
}

// FileMultiple is a file <input multiple> field.
type FileMultiple struct {
	value []*FileHeader
	error string
}

// SetError sets the field's error.
func (e *FileMultiple) SetError(err string) { e.error = err }

// HasError reports whether the field has an error.
func (e *FileMultiple) HasError() bool { return e.error != "" }

// GetError returns the field's error.
func (e *FileMultiple) GetError() string { return e.error }

// GetValue returns the uploaded files.
func (e *FileMultiple) GetValue() []*FileHeader { return e.value }

// IsNotNull reports whether Process ran for a multipart form, even one without files.
func (e *FileMultiple) IsNotNull() bool { return e.value != nil }

// Process reads the field from the parsed form in r. Generated code calls it.
//
// Generated code only.
func (e *FileMultiple) Process(r *http.Request, name string, isMultipart, isRequired bool) {
	if isMultipart {
		fs := files(r, name, isMultipart)
		e.value = make([]*FileHeader, len(fs))
		for i, f := range fs {
			e.value[i] = &FileHeader{f}
		}
	}

	if isRequired && len(e.value) == 0 {
		e.error = MessageRequiredField
		return
	}
}
