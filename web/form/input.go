package form

import "net/http"

// Input is a single-valued <input> field.
type Input[T ElementValueType] struct {
	value     T
	notNull   bool
	formValue string
	error     string
}

// SetError sets the field's error.
func (e *Input[T]) SetError(err string) { e.error = err }

// HasError reports whether the field has an error.
func (e *Input[T]) HasError() bool { return e.error != "" }

// GetError returns the field's error.
func (e *Input[T]) GetError() string { return e.error }

// GetValue returns the field's value.
func (e *Input[T]) GetValue() T { return e.value }

// GetFormValue returns the text posted for the field, so a value that did not parse can be
// shown again.
func (e *Input[T]) GetFormValue() string { return e.formValue }

// IsNotNull reports whether the field has a value, set or posted.
func (e *Input[T]) IsNotNull() bool { return e.notNull }

// SetValue sets the field's value.
func (e *Input[T]) SetValue(v T) {
	e.value = v
	e.notNull = true
}

// Process reads the field from the parsed form in r. Generated code calls it.
//
// Generated code only.
func (e *Input[T]) Process(r *http.Request, name string, isMultipart, isRequired bool) {
	vs := values(r, name, isMultipart)

	if len(vs) > 0 {
		e.formValue = vs[0]
		e.notNull = true
	}

	if isRequired && e.formValue == "" {
		e.error = MessageRequiredField
		return
	}

	if e.formValue != "" {
		parseValue(e.formValue, &e.value, &e.error)
	}
}

// InputMultiple is an <input> field that takes several values, such as a group of checkboxes
// sharing a name.
type InputMultiple[T ElementValueType] struct {
	value     map[T]struct{}
	formValue string
	error     string
}

// SetError sets the field's error.
func (e *InputMultiple[T]) SetError(err string) { e.error = err }

// HasError reports whether the field has an error.
func (e *InputMultiple[T]) HasError() bool { return e.error != "" }

// GetError returns the field's error.
func (e *InputMultiple[T]) GetError() string { return e.error }

// GetValue returns the field's values.
func (e *InputMultiple[T]) GetValue() map[T]struct{} { return e.value }

// GetFormValue returns "": a multi-valued field keeps no posted text.
func (e *InputMultiple[T]) GetFormValue() string { return e.formValue }

// IsNotNull reports whether the field has values, set or posted.
func (e *InputMultiple[T]) IsNotNull() bool { return e.value != nil }

// SetValue sets the field's values.
func (e *InputMultiple[T]) SetValue(v map[T]struct{}) { e.value = v }

// Process reads the field from the parsed form in r. Generated code calls it.
//
// Generated code only.
func (e *InputMultiple[T]) Process(r *http.Request, name string, isMultipart, isRequired bool) {
	vs := values(r, name, isMultipart)

	if len(vs) > 0 {
		e.value = make(map[T]struct{})
	}

	for _, formValue := range vs {
		var (
			parsedValue T
			err         string
		)
		parseValue(formValue, &parsedValue, &err)
		if err != "" {
			e.error = err
			return
		}
		e.value[parsedValue] = struct{}{}
	}

	if isRequired && len(e.value) == 0 {
		e.error = MessageRequiredField
		return
	}
}
