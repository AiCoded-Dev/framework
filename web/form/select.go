package form

import "net/http"

// Select is a single-valued <select> field. A posted value must be one of its options.
type Select[T ElementValueType] struct {
	value   T
	notNull bool
	error   string
	options []SelectOptionElement[T]
}

// SetOptions sets the options a viewer can choose from.
func (e *Select[T]) SetOptions(o []SelectOptionElement[T]) { e.options = o }

// GetOptions returns the field's options.
func (e *Select[T]) GetOptions() []SelectOptionElement[T] { return e.options }

// SetError sets the field's error.
func (e *Select[T]) SetError(err string) { e.error = err }

// HasError reports whether the field has an error.
func (e *Select[T]) HasError() bool { return e.error != "" }

// GetError returns the field's error.
func (e *Select[T]) GetError() string { return e.error }

// GetValue returns the field's value.
func (e *Select[T]) GetValue() T { return e.value }

// IsNotNull reports whether the field has a value, set or posted.
func (e *Select[T]) IsNotNull() bool { return e.notNull }

// SetValue sets the field's value.
func (e *Select[T]) SetValue(v T) {
	e.value = v
	e.notNull = true
}

// Process reads the field from the parsed form in r. Generated code calls it after the
// form's options are set.
//
// Generated code only.
func (e *Select[T]) Process(r *http.Request, name string, isMultipart, isRequired bool) {
	vs := values(r, name, isMultipart)

	var formValue = ""
	if len(vs) > 0 {
		formValue = vs[0]
		e.notNull = true
	}

	if isRequired && formValue == "" {
		e.error = MessageRequiredField
		return
	}

	if formValue != "" {
		var v T
		parseValue(formValue, &v, &e.error)
		if e.error == "" && !offered(e.options, v) {
			e.error = MessageInvalidOption
		}
		if e.error == "" {
			e.value = v
		}
	}
}

// SelectMultiple is a <select multiple> field. Every posted value must be one of its options.
type SelectMultiple[T ElementValueType] struct {
	value   map[T]struct{}
	error   string
	options []SelectOptionElement[T]
}

// SetOptions sets the options a viewer can choose from.
func (e *SelectMultiple[T]) SetOptions(o []SelectOptionElement[T]) { e.options = o }

// GetOptions returns the field's options.
func (e *SelectMultiple[T]) GetOptions() []SelectOptionElement[T] { return e.options }

// SetError sets the field's error.
func (e *SelectMultiple[T]) SetError(err string) { e.error = err }

// HasError reports whether the field has an error.
func (e *SelectMultiple[T]) HasError() bool { return e.error != "" }

// GetError returns the field's error.
func (e *SelectMultiple[T]) GetError() string { return e.error }

// GetValue returns the field's values.
func (e *SelectMultiple[T]) GetValue() map[T]struct{} { return e.value }

// IsNotNull reports whether the field has values, set or posted.
func (e *SelectMultiple[T]) IsNotNull() bool { return e.value != nil }

// SetValue sets the field's values.
func (e *SelectMultiple[T]) SetValue(v map[T]struct{}) { e.value = v }

// Process reads the field from the parsed form in r. Generated code calls it after the
// form's options are set.
//
// Generated code only.
func (e *SelectMultiple[T]) Process(r *http.Request, name string, isMultipart, isRequired bool) {
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
		if err == "" && !offered(e.options, parsedValue) {
			err = MessageInvalidOption
		}
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
