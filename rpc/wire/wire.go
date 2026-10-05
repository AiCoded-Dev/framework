// Package wire encodes and decodes the protobuf wire format for the code aicoded generate
// writes for calls between apps. App code does not use it.
package wire

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"time"
	"unicode/utf8"

	"google.golang.org/protobuf/encoding/protowire"
)

// MaxDepth bounds how deeply messages may nest inside the message Decode reads.
const MaxDepth = 1000

var (
	errType  = errors.New("wire: a field has the wrong wire type")
	errUTF8  = errors.New("wire: a string is not valid UTF-8")
	errNanos = errors.New("wire: a time's nanoseconds are out of range")
	errDepth = errors.New("wire: messages nest too deeply")
)

// AppendBool appends field n holding v.
func AppendBool(b []byte, n protowire.Number, v bool) []byte {
	return protowire.AppendVarint(protowire.AppendTag(b, n, protowire.VarintType), protowire.EncodeBool(v))
}

// AppendInt32 appends field n holding v.
func AppendInt32(b []byte, n protowire.Number, v int32) []byte {
	return AppendInt64(b, n, int64(v))
}

// AppendInt64 appends field n holding v.
func AppendInt64(b []byte, n protowire.Number, v int64) []byte {
	return protowire.AppendVarint(protowire.AppendTag(b, n, protowire.VarintType), unsigned(v))
}

// AppendUint32 appends field n holding v.
func AppendUint32(b []byte, n protowire.Number, v uint32) []byte {
	return AppendUint64(b, n, uint64(v))
}

// AppendUint64 appends field n holding v.
func AppendUint64(b []byte, n protowire.Number, v uint64) []byte {
	return protowire.AppendVarint(protowire.AppendTag(b, n, protowire.VarintType), v)
}

// AppendFloat32 appends field n holding v.
func AppendFloat32(b []byte, n protowire.Number, v float32) []byte {
	return protowire.AppendFixed32(protowire.AppendTag(b, n, protowire.Fixed32Type), math.Float32bits(v))
}

// AppendFloat64 appends field n holding v.
func AppendFloat64(b []byte, n protowire.Number, v float64) []byte {
	return protowire.AppendFixed64(protowire.AppendTag(b, n, protowire.Fixed64Type), math.Float64bits(v))
}

// AppendString appends field n holding v.
func AppendString(b []byte, n protowire.Number, v string) []byte {
	return protowire.AppendString(protowire.AppendTag(b, n, protowire.BytesType), v)
}

// AppendBytes appends field n holding v.
func AppendBytes(b []byte, n protowire.Number, v []byte) []byte {
	return protowire.AppendBytes(protowire.AppendTag(b, n, protowire.BytesType), v)
}

// AppendTime appends field n holding v as the message {1: seconds, 2: nanoseconds} since the
// Unix epoch in UTC.
func AppendTime(b []byte, n protowire.Number, v time.Time) []byte {
	return AppendMessage(b, n, func(b []byte) []byte {
		if s := v.Unix(); s != 0 {
			b = AppendInt64(b, 1, s)
		}
		if ns := v.Nanosecond(); ns != 0 {
			b = AppendInt64(b, 2, int64(ns))
		}
		return b
	})
}

// AppendMessage appends field n holding the message enc appends.
func AppendMessage(b []byte, n protowire.Number, enc func([]byte) []byte) []byte {
	return protowire.AppendBytes(protowire.AppendTag(b, n, protowire.BytesType), enc(nil))
}

// Value is the value of one field. Its accessors refuse a wire type that does not fit.
type Value struct {
	typ   protowire.Type
	x     uint64 // varint and fixed values
	b     []byte // length-delimited values
	depth int
}

// Decode calls f for every field of the message b, in order. f skips a field by not reading
// its value. Decode refuses truncated input and field number 0.
func Decode(b []byte, f func(n protowire.Number, v Value) error) error {
	return decode(b, 0, f)
}

func decode(b []byte, depth int, f func(n protowire.Number, v Value) error) error {
	for len(b) > 0 {
		n, typ, l := protowire.ConsumeTag(b)
		if l < 0 {
			return fmt.Errorf("wire: %w", protowire.ParseError(l))
		}
		b = b[l:]
		v := Value{typ: typ, depth: depth}
		switch typ {
		case protowire.VarintType:
			v.x, l = protowire.ConsumeVarint(b)
		case protowire.Fixed32Type:
			var x uint32
			x, l = protowire.ConsumeFixed32(b)
			v.x = uint64(x)
		case protowire.Fixed64Type:
			v.x, l = protowire.ConsumeFixed64(b)
		case protowire.BytesType:
			v.b, l = protowire.ConsumeBytes(b)
		default:
			l = protowire.ConsumeFieldValue(n, typ, b)
		}
		if l < 0 {
			return fmt.Errorf("wire: %w", protowire.ParseError(l))
		}
		b = b[l:]
		if err := f(n, v); err != nil {
			return err
		}
	}
	return nil
}

func (v Value) varint() (uint64, error) {
	if v.typ != protowire.VarintType {
		return 0, errType
	}
	return v.x, nil
}

// Bool returns a bool value.
func (v Value) Bool() (bool, error) {
	x, err := v.varint()
	return protowire.DecodeBool(x), err
}

// Int32 returns an int32 value.
func (v Value) Int32() (int32, error) {
	x, err := v.varint()
	return signed32(x), err
}

// Int64 returns an int64 value.
func (v Value) Int64() (int64, error) {
	x, err := v.varint()
	return signed(x), err
}

// Uint32 returns a uint32 value.
func (v Value) Uint32() (uint32, error) {
	x, err := v.varint()
	return low32(x), err
}

// Uint64 returns a uint64 value.
func (v Value) Uint64() (uint64, error) {
	return v.varint()
}

// Float32 returns a float32 value.
func (v Value) Float32() (float32, error) {
	if v.typ != protowire.Fixed32Type {
		return 0, errType
	}
	return math.Float32frombits(low32(v.x)), nil
}

// Float64 returns a float64 value.
func (v Value) Float64() (float64, error) {
	if v.typ != protowire.Fixed64Type {
		return 0, errType
	}
	return math.Float64frombits(v.x), nil
}

// String returns a string value; it must be valid UTF-8.
func (v Value) String() (string, error) {
	if v.typ != protowire.BytesType {
		return "", errType
	}
	if !utf8.Valid(v.b) {
		return "", errUTF8
	}
	return string(v.b), nil
}

// Bytes returns a copy of a bytes value.
func (v Value) Bytes() ([]byte, error) {
	if v.typ != protowire.BytesType {
		return nil, errType
	}
	return bytes.Clone(v.b), nil
}

// Time returns a time value, in UTC.
func (v Value) Time() (time.Time, error) {
	var s, ns int64
	err := v.Decode(func(n protowire.Number, f Value) (err error) {
		switch n {
		case 1:
			s, err = f.Int64()
		case 2:
			ns, err = f.Int64()
		}
		return err
	})
	if err != nil {
		return time.Time{}, err
	}
	if ns < 0 || ns >= 1e9 {
		return time.Time{}, errNanos
	}
	return time.Unix(s, ns).UTC(), nil
}

// Message returns the encoded message a message value holds. Generated code reads nested
// messages with Decode instead, which bounds their depth.
func (v Value) Message() ([]byte, error) {
	if v.typ != protowire.BytesType {
		return nil, errType
	}
	return v.b, nil
}

// Decode calls f for every field of the message v holds, like the package's Decode. It refuses
// a message nested more than MaxDepth levels inside the one Decode started with.
func (v Value) Decode(f func(n protowire.Number, v Value) error) error {
	if v.typ != protowire.BytesType {
		return errType
	}
	if v.depth >= MaxDepth {
		return errDepth
	}
	return decode(v.b, v.depth+1, f)
}

// Packed calls f for every element of a repeated scalar field whose elements have wire type
// elem: once when v is a single element, and for each element when v holds them packed.
func (v Value) Packed(elem protowire.Type, f func(Value) error) error {
	if elem != protowire.VarintType && elem != protowire.Fixed32Type && elem != protowire.Fixed64Type {
		return errType
	}
	if v.typ == elem {
		return f(v)
	}
	if v.typ != protowire.BytesType {
		return errType
	}
	for b := v.b; len(b) > 0; {
		e := Value{typ: elem, depth: v.depth}
		var l int
		switch elem {
		case protowire.VarintType:
			e.x, l = protowire.ConsumeVarint(b)
		case protowire.Fixed32Type:
			var x uint32
			x, l = protowire.ConsumeFixed32(b)
			e.x = uint64(x)
		default:
			e.x, l = protowire.ConsumeFixed64(b)
		}
		if l < 0 {
			return fmt.Errorf("wire: %w", protowire.ParseError(l))
		}
		b = b[l:]
		if err := f(e); err != nil {
			return err
		}
	}
	return nil
}

// unsigned returns the two's complement bits of v.
func unsigned(v int64) uint64 {
	u := uint64(v & math.MaxInt64)
	if v < 0 {
		u |= 1 << 63
	}
	return u
}

// signed reads x as a two's complement int64.
func signed(x uint64) int64 {
	i := int64(x & math.MaxInt64)
	if x > math.MaxInt64 {
		i |= math.MinInt64
	}
	return i
}

// low32 returns the low 32 bits of x.
func low32(x uint64) uint32 {
	return uint32(x & math.MaxUint32)
}

// signed32 reads the low 32 bits of x as a two's complement int32.
func signed32(x uint64) int32 {
	i := int32(x & math.MaxInt32)
	if x&(1<<31) != 0 {
		i |= math.MinInt32
	}
	return i
}
