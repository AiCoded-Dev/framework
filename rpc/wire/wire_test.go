package wire_test

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"

	"aicoded.dev/framework/rpc/wire"
)

func ignore(protowire.Number, wire.Value) error { return nil }

// one decodes the single field of b with get.
func one(b []byte, get func(wire.Value) error) error {
	return wire.Decode(b, func(_ protowire.Number, v wire.Value) error { return get(v) })
}

func TestRoundTrip(t *testing.T) {
	when := time.Date(2026, 9, 29, 10, 30, 0, 123456789, time.FixedZone("Lisbon", 3600))
	b := wire.AppendBool(nil, 1, true)
	b = wire.AppendInt32(b, 2, math.MinInt32)
	b = wire.AppendInt64(b, 3, math.MinInt64)
	b = wire.AppendUint32(b, 4, math.MaxUint32)
	b = wire.AppendUint64(b, 5, math.MaxUint64)
	b = wire.AppendFloat32(b, 6, -1.5)
	b = wire.AppendFloat64(b, 7, math.Pi)
	b = wire.AppendString(b, 8, "olá")
	b = wire.AppendBytes(b, 9, []byte{0, 1, 2})
	b = wire.AppendTime(b, 10, when)
	b = wire.AppendTime(b, 11, time.Time{})
	b = wire.AppendTime(b, 12, time.Unix(-1, 999_999_999))
	b = wire.AppendTime(b, 13, time.Unix(0, 0))
	b = wire.AppendMessage(b, 14, func(b []byte) []byte { return wire.AppendInt32(b, 1, -7) })

	got := map[protowire.Number]any{}
	require.NoError(t, wire.Decode(b, func(n protowire.Number, v wire.Value) error {
		var x any
		var err error
		switch n {
		case 1:
			x, err = v.Bool()
		case 2:
			x, err = v.Int32()
		case 3:
			x, err = v.Int64()
		case 4:
			x, err = v.Uint32()
		case 5:
			x, err = v.Uint64()
		case 6:
			x, err = v.Float32()
		case 7:
			x, err = v.Float64()
		case 8:
			x, err = v.String()
		case 9:
			x, err = v.Bytes()
		case 14:
			var inner int32
			err = v.Decode(func(_ protowire.Number, v wire.Value) (err error) {
				inner, err = v.Int32()
				return err
			})
			x = inner
		default:
			x, err = v.Time()
		}
		got[n] = x
		return err
	}))
	assert.Equal(t, map[protowire.Number]any{
		1: true, 2: int32(math.MinInt32), 3: int64(math.MinInt64), 4: uint32(math.MaxUint32), 5: uint64(math.MaxUint64),
		6: float32(-1.5), 7: math.Pi, 8: "olá", 9: []byte{0, 1, 2},
		10: when.UTC(), 11: time.Time{}, 12: time.Unix(-1, 999_999_999).UTC(), 13: time.Unix(0, 0).UTC(), 14: int32(-7),
	}, got)
}

func TestBytesIsACopy(t *testing.T) {
	b := wire.AppendBytes(nil, 1, []byte("abc"))
	var got []byte
	require.NoError(t, one(b, func(v wire.Value) (err error) {
		got, err = v.Bytes()
		return err
	}))
	b[len(b)-1] = 'x'
	assert.Equal(t, []byte("abc"), got)
}

func TestPacked(t *testing.T) {
	var ints []byte
	for _, x := range []int64{7, -3, 0} {
		ints = protowire.AppendVarint(ints, uint64(x))
	}
	b := protowire.AppendBytes(protowire.AppendTag(nil, 1, protowire.BytesType), ints)
	b = wire.AppendInt64(b, 1, 9)
	b = wire.AppendInt64(b, 1, 0)
	var got []int64
	require.NoError(t, one(b, func(v wire.Value) error {
		return v.Packed(protowire.VarintType, func(e wire.Value) error {
			x, err := e.Int64()
			got = append(got, x)
			return err
		})
	}))
	assert.Equal(t, []int64{7, -3, 0, 9, 0}, got, "packed and unpacked elements, zeros included")

	floats := protowire.AppendFixed32(protowire.AppendFixed32(nil, math.Float32bits(1.5)), math.Float32bits(-2))
	var fs []float32
	require.NoError(t, one(wire.AppendBytes(nil, 1, floats), func(v wire.Value) error {
		return v.Packed(protowire.Fixed32Type, func(e wire.Value) error {
			x, err := e.Float32()
			fs = append(fs, x)
			return err
		})
	}))
	assert.Equal(t, []float32{1.5, -2}, fs)

	skip := func(wire.Value) error { return nil }
	require.Error(t, one(wire.AppendBytes(nil, 1, []byte{0x80}), func(v wire.Value) error { return v.Packed(protowire.VarintType, skip) }), "truncated element")
	require.Error(t, one(wire.AppendFloat64(nil, 1, 1), func(v wire.Value) error { return v.Packed(protowire.VarintType, skip) }), "wrong element type")
	require.Error(t, one(wire.AppendBytes(nil, 1, nil), func(v wire.Value) error { return v.Packed(protowire.BytesType, skip) }), "not a scalar")
}

func TestDecodeSkipsUnknownFields(t *testing.T) {
	b := wire.AppendFloat64(nil, 7, 1)
	b = protowire.AppendTag(b, 8, protowire.StartGroupType)
	b = wire.AppendString(b, 1, "in a group")
	b = protowire.AppendTag(b, 8, protowire.EndGroupType)
	b = wire.AppendString(b, 1, "known")
	var got string
	require.NoError(t, wire.Decode(b, func(n protowire.Number, v wire.Value) (err error) {
		if n == 1 {
			got, err = v.String()
		}
		return err
	}))
	assert.Equal(t, "known", got)
}

func TestDecodeRefuses(t *testing.T) {
	str := func(v wire.Value) error { _, err := v.String(); return err }
	tm := func(v wire.Value) error { _, err := v.Time(); return err }
	nanos := func(ns int64) []byte {
		return wire.AppendMessage(nil, 1, func(b []byte) []byte { return wire.AppendInt64(wire.AppendInt64(b, 1, 5), 2, ns) })
	}
	for name, err := range map[string]error{
		"wrong wire type":    one(wire.AppendInt64(nil, 1, 5), str),
		"truncated varint":   wire.Decode([]byte{0x08, 0x80}, ignore),
		"truncated length":   wire.Decode([]byte{0x0a, 0x05, 'a'}, ignore),
		"truncated tag":      wire.Decode([]byte{0x80}, ignore),
		"bad UTF-8":          one(wire.AppendBytes(nil, 1, []byte{0xff, 0xfe}), str),
		"nanos too large":    one(nanos(1e9), tm),
		"negative nanos":     one(nanos(-1), tm),
		"field 0":            wire.Decode([]byte{0x00, 0x01}, ignore),
		"end group alone":    wire.Decode(protowire.AppendTag(nil, 1, protowire.EndGroupType), ignore),
		"unterminated group": wire.Decode(protowire.AppendTag(nil, 1, protowire.StartGroupType), ignore),
	} {
		require.Error(t, err, name)
	}
}

func TestAccessorsCheckTheWireType(t *testing.T) {
	fields := map[string][]byte{
		"varint":  wire.AppendInt64(nil, 1, 1),
		"fixed32": wire.AppendFloat32(nil, 1, 1),
		"fixed64": wire.AppendFloat64(nil, 1, 1),
		"bytes":   wire.AppendBytes(nil, 1, nil),
	}
	accessors := map[string]struct {
		fits string
		read func(wire.Value) error
	}{
		"Bool":    {"varint", func(v wire.Value) error { _, err := v.Bool(); return err }},
		"Int32":   {"varint", func(v wire.Value) error { _, err := v.Int32(); return err }},
		"Int64":   {"varint", func(v wire.Value) error { _, err := v.Int64(); return err }},
		"Uint32":  {"varint", func(v wire.Value) error { _, err := v.Uint32(); return err }},
		"Uint64":  {"varint", func(v wire.Value) error { _, err := v.Uint64(); return err }},
		"Float32": {"fixed32", func(v wire.Value) error { _, err := v.Float32(); return err }},
		"Float64": {"fixed64", func(v wire.Value) error { _, err := v.Float64(); return err }},
		"String":  {"bytes", func(v wire.Value) error { _, err := v.String(); return err }},
		"Bytes":   {"bytes", func(v wire.Value) error { _, err := v.Bytes(); return err }},
		"Time":    {"bytes", func(v wire.Value) error { _, err := v.Time(); return err }},
		"Message": {"bytes", func(v wire.Value) error { _, err := v.Message(); return err }},
		"Decode":  {"bytes", func(v wire.Value) error { return v.Decode(ignore) }},
	}
	for kind, b := range fields {
		for name, a := range accessors {
			err := one(b, a.read)
			if kind == a.fits {
				require.NoError(t, err, "%s of a %s field", name, kind)
			} else {
				require.Error(t, err, "%s of a %s field", name, kind)
			}
		}
	}
}

func TestDecodeBoundsDepth(t *testing.T) {
	nest := func(levels int) []byte {
		var b []byte
		for range levels {
			inner := b
			b = wire.AppendMessage(nil, 1, func(x []byte) []byte { return append(x, inner...) })
		}
		return b
	}
	var walk func(protowire.Number, wire.Value) error
	walk = func(_ protowire.Number, v wire.Value) error { return v.Decode(walk) }
	require.NoError(t, wire.Decode(nest(wire.MaxDepth), walk))
	require.Error(t, wire.Decode(nest(wire.MaxDepth+1), walk))
}

func FuzzDecode(f *testing.F) {
	f.Add(wire.AppendTime(wire.AppendString(nil, 1, "a"), 2, time.Now()))
	f.Add([]byte{0x0a, 0x02, 0x08, 0x01})
	f.Add([]byte{0x0b, 0x0c})
	var walk func(protowire.Number, wire.Value) error
	walk = func(_ protowire.Number, v wire.Value) error {
		_, _ = v.Bool()
		_, _ = v.Int32()
		_, _ = v.Uint32()
		_, _ = v.Float32()
		_, _ = v.Float64()
		_, _ = v.String()
		_, _ = v.Bytes()
		_, _ = v.Time()
		_, _ = v.Message()
		for _, elem := range []protowire.Type{protowire.VarintType, protowire.Fixed32Type, protowire.Fixed64Type} {
			_ = v.Packed(elem, func(e wire.Value) error { _, err := e.Uint64(); return err })
		}
		_ = v.Decode(walk)
		return nil
	}
	f.Fuzz(func(_ *testing.T, b []byte) {
		_ = wire.Decode(b, walk)
	})
}
