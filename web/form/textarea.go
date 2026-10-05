package form

import "net/http"

// Textarea is a <textarea> field.
type Textarea struct {
	value   string
	notNull bool
	error   string
}

// SetError sets the field's error.
func (e *Textarea) SetError(err string) { e.error = err }

// HasError reports whether the field has an error.
func (e *Textarea) HasError() bool { return e.error != "" }

// GetError returns the field's error.
func (e *Textarea) GetError() string { return e.error }

// GetValue returns the field's text.
func (e *Textarea) GetValue() string { return e.value }

// IsNotNull reports whether the field has a value, set or posted.
func (e *Textarea) IsNotNull() bool { return e.notNull }

// SetValue sets the field's text.
func (e *Textarea) SetValue(v string) {
	e.value = v
	e.notNull = true
}

// Process reads the field from the parsed form in r. Generated code calls it.
//
// Generated code only.
func (e *Textarea) Process(r *http.Request, name string, isMultipart, isRequired bool) {
	vs := values(r, name, isMultipart)

	if len(vs) > 0 {
		e.value = vs[0]
		e.notNull = true
	}

	if isRequired && e.value == "" {
		e.error = MessageRequiredField
		return
	}
}
