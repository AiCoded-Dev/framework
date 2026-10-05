package form

import (
	"fmt"
	"html"
	"io"
	"strconv"
)

var (
	_ SelectOptionElement[string] = SelectOption[string]{}
	_ SelectOptionElement[string] = SelectOptionGroup[string]{}
)

// SelectOptionElement is an option or a group of options of a select field.
type SelectOptionElement[T ElementValueType] interface {
	// WriteHtml writes the element's markup, marking the options for which isSelected holds.
	WriteHtml(w io.Writer, isSelected func(v T) bool) error

	// Allows reports whether v is an option a viewer can choose.
	Allows(v T) bool
}

// SelectOption is one option of a select field.
type SelectOption[T ElementValueType] struct {
	Value    T
	Label    string
	Disabled bool
}

// WriteHtml writes the option as an <option> element.
func (o SelectOption[T]) WriteHtml(w io.Writer, isSelected func(v T) bool) error {
	if _, err := io.WriteString(w, `<option value="`); err != nil {
		return err
	}

	if err := writeEscaped(w, o.Value); err != nil {
		return err
	}

	if _, err := io.WriteString(w, `"`); err != nil {
		return err
	}

	if o.Disabled {
		if _, err := io.WriteString(w, " disabled"); err != nil {
			return err
		}
	}

	if isSelected(o.Value) {
		if _, err := io.WriteString(w, " selected"); err != nil {
			return err
		}
	}

	if _, err := io.WriteString(w, `>`); err != nil {
		return err
	}

	if o.Label != "" {
		if err := writeEscaped(w, o.Label); err != nil {
			return err
		}
	}

	if _, err := io.WriteString(w, `</option>`); err != nil {
		return err
	}

	return nil
}

// Allows reports whether v is this option's value and the option is enabled.
func (o SelectOption[T]) Allows(v T) bool { return !o.Disabled && o.Value == v }

// SelectOptionGroup is a labelled group of options of a select field.
type SelectOptionGroup[T ElementValueType] struct {
	Label    string
	Disabled bool
	Options  []SelectOptionElement[T]
}

// WriteHtml writes the group as an <optgroup> element.
func (o SelectOptionGroup[T]) WriteHtml(w io.Writer, isSelected func(v T) bool) error {
	if _, err := io.WriteString(w, `<optgroup label="`); err != nil {
		return err
	}

	if err := writeEscaped(w, o.Label); err != nil {
		return err
	}

	if _, err := io.WriteString(w, `"`); err != nil {
		return err
	}

	if o.Disabled {
		if _, err := io.WriteString(w, " disabled"); err != nil {
			return err
		}
	}

	if _, err := io.WriteString(w, `>`); err != nil {
		return err
	}

	for _, opt := range o.Options {
		if err := opt.WriteHtml(w, isSelected); err != nil {
			return err
		}
	}

	if _, err := io.WriteString(w, `</optgroup>`); err != nil {
		return err
	}

	return nil
}

// Allows reports whether the group is enabled and one of its options allows v.
func (o SelectOptionGroup[T]) Allows(v T) bool { return !o.Disabled && offered(o.Options, v) }

// offered reports whether one of options allows v.
func offered[T ElementValueType](options []SelectOptionElement[T], v T) bool {
	for _, o := range options {
		if o.Allows(v) {
			return true
		}
	}
	return false
}

// writeEscaped writes v as text that is safe in element text and in a double-quoted
// attribute value.
func writeEscaped(w io.Writer, v any) error {
	_, err := io.WriteString(w, html.EscapeString(format(v)))
	return err
}

func format(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case int:
		return strconv.Itoa(x)
	case int8:
		return strconv.FormatInt(int64(x), 10)
	case int16:
		return strconv.FormatInt(int64(x), 10)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case int64:
		return strconv.FormatInt(x, 10)
	case uint:
		return strconv.FormatUint(uint64(x), 10)
	case uint8:
		return strconv.FormatUint(uint64(x), 10)
	case uint16:
		return strconv.FormatUint(uint64(x), 10)
	case uint32:
		return strconv.FormatUint(uint64(x), 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	case float32:
		return strconv.FormatFloat(float64(x), 'g', -1, 32)
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	default:
		return fmt.Sprint(v)
	}
}
