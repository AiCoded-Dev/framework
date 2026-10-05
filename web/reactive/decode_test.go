package reactive

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecode(t *testing.T) {
	n, err := Decode[int](json.RawMessage(`42`))
	require.NoError(t, err)
	assert.Equal(t, 42, n)

	n, err = Decode[int](json.RawMessage(`"43"`))
	require.NoError(t, err)
	assert.Equal(t, 43, n, "native inputs send strings")

	s, err := Decode[string](json.RawMessage(`"<b>"`))
	require.NoError(t, err)
	assert.Equal(t, "<b>", s)

	type pair struct{ A, B int }
	p, err := Decode[pair](json.RawMessage(`{"A":1,"B":2}`))
	require.NoError(t, err)
	assert.Equal(t, pair{1, 2}, p)

	_, err = Decode[int](json.RawMessage(`"x"`))
	require.Error(t, err)
	_, err = Decode[uint8](json.RawMessage(`300`))
	require.Error(t, err)
}

// Only bools and numbers are parsed from text; other types whose JSON is a string decode as
// JSON.
func TestDecodeStringTypes(t *testing.T) {
	type status string
	st, err := Decode[status](json.RawMessage(`"open"`))
	require.NoError(t, err)
	assert.Equal(t, status("open"), st)

	at, err := Decode[time.Time](json.RawMessage(`"2026-09-27T10:00:00Z"`))
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC), at)
}

// A value sent as text must fit its type exactly as a JSON number must.
func TestDecodeRefusesTextThatDoesNotFit(t *testing.T) {
	_, err := Decode[uint8](json.RawMessage(`"300"`))
	require.Error(t, err)
	_, err = Decode[float64](json.RawMessage(`"NaN"`))
	require.Error(t, err)
	_, err = Decode[int](json.RawMessage(``))
	require.Error(t, err)
}

func parsed[T any](t *testing.T, s string, want T) {
	t.Helper()
	v, err := ParseValue[T](s)
	require.NoError(t, err)
	assert.Equal(t, want, v)
}

func TestParseValue(t *testing.T) {
	parsed(t, "<b>", "<b>")
	parsed(t, "on", true)
	parsed(t, "", false)
	parsed(t, "-5", -5)
	parsed(t, "-128", int8(math.MinInt8))
	parsed(t, "32767", int16(math.MaxInt16))
	parsed(t, "-2147483648", int32(math.MinInt32))
	parsed(t, "9223372036854775807", int64(math.MaxInt64))
	parsed(t, "5", uint(5))
	parsed(t, "255", uint8(math.MaxUint8))
	parsed(t, "65535", uint16(math.MaxUint16))
	parsed(t, "4294967295", uint32(math.MaxUint32))
	parsed(t, "18446744073709551615", uint64(math.MaxUint64))
	parsed(t, "1.5", float32(1.5))
	parsed(t, "-2.25", -2.25)
}

func errOf[T any](_ T, err error) error { return err }

func TestParseValueRefuses(t *testing.T) {
	for name, err := range map[string]error{
		"bool maybe":       errOf(ParseValue[bool]("maybe")),
		"int x":            errOf(ParseValue[int]("x")),
		"int8 128":         errOf(ParseValue[int8]("128")),
		"int16 -32769":     errOf(ParseValue[int16]("-32769")),
		"int32 2147483648": errOf(ParseValue[int32]("2147483648")),
		"uint -1":          errOf(ParseValue[uint]("-1")),
		"uint8 256":        errOf(ParseValue[uint8]("256")),
		"uint16 65536":     errOf(ParseValue[uint16]("65536")),
		"uint32 2^32":      errOf(ParseValue[uint32]("4294967296")),
		"float32 1e39":     errOf(ParseValue[float32]("1e39")),
		"float64 Inf":      errOf(ParseValue[float64]("Inf")),
		"float64 NaN":      errOf(ParseValue[float64]("NaN")),
		"unsupported type": errOf(ParseValue[[]int]("1")),
	} {
		assert.Error(t, err, name)
	}
}

// Errors never repeat the value, which came from the viewer.
func TestErrorsLeaveTheValueOut(t *testing.T) {
	for name, err := range map[string]error{
		"int from text":    errOf(Decode[int](json.RawMessage(`"secret"`))),
		"uint8 from text":  errOf(Decode[uint8](json.RawMessage(`"98765"`))),
		"uint8 from JSON":  errOf(Decode[uint8](json.RawMessage(`98765`))),
		"bool from text":   errOf(Decode[bool](json.RawMessage(`"secret"`))),
		"float from text":  errOf(Decode[float64](json.RawMessage(`"secret"`))),
		"struct field":     errOf(Decode[struct{ A int }](json.RawMessage(`{"A":"secret"}`))),
		"time":             errOf(Decode[time.Time](json.RawMessage(`"secret"`))),
		"broken JSON":      errOf(Decode[[]string](json.RawMessage(`["secret"`))),
		"ParseValue range": errOf(ParseValue[int8]("98765")),
	} {
		require.Error(t, err, name)
		assert.NotContains(t, err.Error(), "secret", name)
		assert.NotContains(t, err.Error(), "98765", name)
	}
	require.EqualError(t, errOf(ParseValue[uint8]("256")), "reactive: cannot read uint8: value out of range")
	require.EqualError(t, errOf(ParseValue[int]("x")), "reactive: cannot read int: invalid syntax")
	require.EqualError(t, errOf(Decode[int](json.RawMessage(`true`))), "reactive: cannot read int: wrong type")
	require.EqualError(t, errOf(ParseValue[[]int]("1")), "reactive: cannot read []int: unsupported type")
}
