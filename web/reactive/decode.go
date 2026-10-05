package reactive

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
)

// Errors of Decode and ParseValue. They never repeat the value, which came from the viewer.
var (
	errSyntax      = errors.New("invalid syntax")
	errRange       = errors.New("value out of range")
	errType        = errors.New("wrong type")
	errUnsupported = errors.New("unsupported type")
)

// Decode reads a written value. Native inputs send strings, so a JSON string is parsed into a
// bool or number T; anything else is JSON for T.
//
// Generated code only.
func Decode[T any](raw json.RawMessage) (T, error) {
	var v T
	if len(raw) > 0 && raw[0] == '"' && boolOrNumber(v) {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return v, valueError(v, err)
		}
		return ParseValue[T](s)
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		var zero T
		return zero, valueError(v, err)
	}
	return v, nil
}

func boolOrNumber(v any) bool {
	switch v.(type) {
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return true
	}
	return false
}

// ParseValue parses text a page sent into T: a string, a bool or a number. A number must fit
// T, and a float must be finite.
func ParseValue[T any](s string) (T, error) {
	var v T
	var err error
	switch p := any(&v).(type) {
	case *string:
		*p = s
	case *bool:
		*p, err = parseBool(s)
	case *int:
		*p, err = strconv.Atoi(s)
	case *int8:
		var n int64
		if n, err = strconv.ParseInt(s, 10, 8); err == nil {
			*p = int8(n)
		}
	case *int16:
		var n int64
		if n, err = strconv.ParseInt(s, 10, 16); err == nil {
			*p = int16(n)
		}
	case *int32:
		var n int64
		if n, err = strconv.ParseInt(s, 10, 32); err == nil {
			*p = int32(n)
		}
	case *int64:
		*p, err = strconv.ParseInt(s, 10, 64)
	case *uint:
		var n uint64
		if n, err = strconv.ParseUint(s, 10, strconv.IntSize); err == nil {
			*p = uint(n)
		}
	case *uint8:
		var n uint64
		if n, err = strconv.ParseUint(s, 10, 8); err == nil {
			*p = uint8(n)
		}
	case *uint16:
		var n uint64
		if n, err = strconv.ParseUint(s, 10, 16); err == nil {
			*p = uint16(n)
		}
	case *uint32:
		var n uint64
		if n, err = strconv.ParseUint(s, 10, 32); err == nil {
			*p = uint32(n)
		}
	case *uint64:
		*p, err = strconv.ParseUint(s, 10, 64)
	case *float32:
		var f float64
		if f, err = parseFloat(s, 32); err == nil {
			*p = float32(f)
		}
	case *float64:
		*p, err = parseFloat(s, 64)
	default:
		return v, fmt.Errorf("reactive: cannot read %T: %w", v, errUnsupported)
	}
	if err != nil {
		var zero T
		return zero, valueError(v, err)
	}
	return v, nil
}

// valueError says why a value of v's type could not be read, without the value itself: the
// errors of strconv and encoding/json quote it.
func valueError(v any, err error) error {
	var typeErr *json.UnmarshalTypeError
	reason := errSyntax
	switch {
	case errors.Is(err, strconv.ErrRange):
		reason = errRange
	case errors.As(err, &typeErr):
		reason = errType
	}
	return fmt.Errorf("reactive: cannot read %T: %w", v, reason)
}

func parseBool(s string) (bool, error) {
	switch s {
	case "true", "1", "on", "yes":
		return true, nil
	case "false", "0", "off", "no", "":
		return false, nil
	default:
		return strconv.ParseBool(s)
	}
}

func parseFloat(s string, bits int) (float64, error) {
	f, err := strconv.ParseFloat(s, bits)
	if err == nil && (math.IsNaN(f) || math.IsInf(f, 0)) {
		return 0, strconv.ErrRange
	}
	return f, err
}
