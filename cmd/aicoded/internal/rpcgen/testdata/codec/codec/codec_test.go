package codec

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"

	"aicoded.dev/framework/rpc/wire"
)

func ptr[T any](v T) *T { return &v }

func tag(b []byte, n protowire.Number, typ protowire.Type) []byte {
	return protowire.AppendTag(b, n, typ)
}

func decode(b []byte) (All, error) {
	var out All
	err := wire.Decode(b, out.aicodedDecode)
	return out, err
}

func TestRoundTrip(t *testing.T) {
	when := time.Date(2026, 9, 29, 10, 30, 0, 5, time.UTC)
	for name, in := range map[string]All{
		"zero": {},
		"scalars": {Bool: true, I32: -7, I64: math.MinInt64, U32: math.MaxUint32, U64: math.MaxUint64,
			F32: -1.5, F64: math.Pi, Str: "Grüße", Raw: []byte{0, 1}, When: when},
		"times around 1970": {When: time.Date(1969, 12, 31, 23, 59, 59, 999_999_999, time.UTC), PWhen: ptr(time.Unix(0, 0).UTC())},
		"pointers to zero":  {PBool: ptr(false), PI64: ptr(int64(0)), PStr: ptr(""), PChild: &Child{Next: &Child{}}},
		"repeated": {Bools: []bool{true, false}, I32s: []int32{0, -1}, I64s: []int64{1 << 40}, U32s: []uint32{0},
			U64s: []uint64{3}, F32s: []float32{0, 2.5}, F64s: []float64{-0.25}, Strs: []string{"", "a"},
			Raws: [][]byte{{0}, {1}}, Whens: []time.Time{{}, when}, Children: []Child{{}, {Name: "b"}}},
		"nested": {Child: Child{Name: "a", Next: &Child{Name: "b"}}},
	} {
		out, err := decode(in.aicodedAppend(nil))
		require.NoError(t, err, name)
		assert.Equal(t, in, out, name)
	}
}

func TestNumbersAndZeroValues(t *testing.T) {
	assert.Empty(t, (&All{}).aicodedAppend(nil), "zero values are left out")
	assert.Equal(t, protowire.AppendVarint(tag(nil, 2, protowire.VarintType), 1), (&All{I32: 1}).aicodedAppend(nil))
	assert.Equal(t, protowire.AppendVarint(tag(nil, 13, protowire.VarintType), 0), (&All{PI64: ptr(int64(0))}).aicodedAppend(nil),
		"a pointer to a zero value is written")
	assert.Equal(t, protowire.AppendString(tag(nil, 2, protowire.BytesType), "x"), (&Child{Name: "x"}).aicodedAppend(nil),
		"numbers come from the schema, not from the order of the fields")
}

func TestDecodeSkipsUnknownFields(t *testing.T) {
	out, err := decode(protowire.AppendVarint(tag((&All{I32: 5}).aicodedAppend(nil), 99, protowire.VarintType), 1))
	require.NoError(t, err)
	assert.Equal(t, All{I32: 5}, out)
}

func TestDecodeStartsNestedValuesAfresh(t *testing.T) {
	b := (&All{Child: Child{Name: "a"}}).aicodedAppend(nil)
	b = (&All{Child: Child{Next: &Child{}}}).aicodedAppend(b)
	out, err := decode(b)
	require.NoError(t, err)
	assert.Equal(t, All{Child: Child{Next: &Child{}}}, out, "the second Child replaces the first")
}

func TestDecodePacked(t *testing.T) {
	var ints, floats []byte
	for _, v := range []int32{1, -2, 3} {
		ints = protowire.AppendVarint(ints, uint64(v))
	}
	floats = protowire.AppendFixed32(floats, math.Float32bits(2.5))
	b := protowire.AppendBytes(tag(nil, 18, protowire.BytesType), ints)
	b = protowire.AppendBytes(tag(b, 22, protowire.BytesType), floats)
	b = protowire.AppendVarint(tag(b, 18, protowire.VarintType), 4)
	out, err := decode(b)
	require.NoError(t, err)
	assert.Equal(t, All{I32s: []int32{1, -2, 3, 4}, F32s: []float32{2.5}}, out)
}

// nested returns a Child whose Next field is set levels deep.
func nested(levels int) []byte {
	var b []byte
	for range levels {
		b = protowire.AppendBytes(tag(nil, 1, protowire.BytesType), b)
	}
	return b
}

func TestDecodeRefusesDeepNesting(t *testing.T) {
	var c Child
	require.NoError(t, wire.Decode(nested(wire.MaxDepth), c.aicodedDecode))
	require.Error(t, wire.Decode(nested(wire.MaxDepth+1), c.aicodedDecode))
}

func TestDecodeRefuses(t *testing.T) {
	for name, b := range map[string][]byte{
		"int32 as bytes":    protowire.AppendString(tag(nil, 2, protowire.BytesType), "x"),
		"string as varint":  protowire.AppendVarint(tag(nil, 8, protowire.VarintType), 1),
		"message as varint": protowire.AppendVarint(tag(nil, 11, protowire.VarintType), 1),
		"truncated":         tag(nil, 2, protowire.VarintType),
		"bad UTF-8":         protowire.AppendString(tag(nil, 8, protowire.BytesType), "\xff"),
		"bad nested":        protowire.AppendBytes(tag(nil, 11, protowire.BytesType), protowire.AppendVarint(tag(nil, 2, protowire.VarintType), 1)),
	} {
		_, err := decode(b)
		require.Error(t, err, name)
	}
}
