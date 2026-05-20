package domain

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
)

// RegisterDecoder decodes typed values out of a slice of 16-bit registers,
// honouring byte and word order configured on the parent Poller.
//
// The underlying storage is the big-endian byte representation of the
// registers (i.e. the same byte sequence that travels on the Modbus wire).
// Per-value decoding then applies the configured byte/word ordering to the
// extracted window before interpreting it as the target type.
type RegisterDecoder struct {
	bytes  []byte
	endian Endian
}

// NewRegisterDecoder creates a decoder from raw 16-bit registers.
func NewRegisterDecoder(regs []uint16, endian Endian) *RegisterDecoder {
	buf := make([]byte, len(regs)*2)
	for i, r := range regs {
		binary.BigEndian.PutUint16(buf[i*2:], r)
	}
	return &RegisterDecoder{bytes: buf, endian: endian}
}

// extract returns the normalized big-endian bytes at the given word offset for
// a value spanning width words. Byte and word order are applied so the result
// can be decoded as plain big-endian.
func (d *RegisterDecoder) extract(wordOffset, width int) ([]byte, error) {
	start := wordOffset * 2
	end := start + width*2
	if start < 0 || end > len(d.bytes) {
		return nil, fmt.Errorf("offset %d width %d out of range (have %d bytes)", wordOffset, width, len(d.bytes))
	}
	out := make([]byte, width*2)
	copy(out, d.bytes[start:end])
	if d.endian.Bytes == LittleEndian {
		for i := 0; i < width; i++ {
			out[i*2], out[i*2+1] = out[i*2+1], out[i*2]
		}
	}
	if d.endian.Words == LittleEndian {
		for i, j := 0, width-1; i < j; i, j = i+1, j-1 {
			out[i*2], out[j*2] = out[j*2], out[i*2]
			out[i*2+1], out[j*2+1] = out[j*2+1], out[i*2+1]
		}
	}
	return out, nil
}

// DecodeAt decodes a typed value at the given word offset.
func (d *RegisterDecoder) DecodeAt(wordOffset int, t DataType) (any, error) {
	switch t {
	case TypeUint16:
		b, err := d.extract(wordOffset, 1)
		if err != nil {
			return nil, err
		}
		return binary.BigEndian.Uint16(b), nil
	case TypeInt16:
		b, err := d.extract(wordOffset, 1)
		if err != nil {
			return nil, err
		}
		return int16(binary.BigEndian.Uint16(b)), nil
	case TypeUint32:
		b, err := d.extract(wordOffset, 2)
		if err != nil {
			return nil, err
		}
		return binary.BigEndian.Uint32(b), nil
	case TypeInt32:
		b, err := d.extract(wordOffset, 2)
		if err != nil {
			return nil, err
		}
		return int32(binary.BigEndian.Uint32(b)), nil
	case TypeUint64:
		b, err := d.extract(wordOffset, 4)
		if err != nil {
			return nil, err
		}
		return binary.BigEndian.Uint64(b), nil
	case TypeInt64:
		b, err := d.extract(wordOffset, 4)
		if err != nil {
			return nil, err
		}
		return int64(binary.BigEndian.Uint64(b)), nil
	case TypeFloat32:
		b, err := d.extract(wordOffset, 2)
		if err != nil {
			return nil, err
		}
		return math.Float32frombits(binary.BigEndian.Uint32(b)), nil
	case TypeFloat64:
		b, err := d.extract(wordOffset, 4)
		if err != nil {
			return nil, err
		}
		return math.Float64frombits(binary.BigEndian.Uint64(b)), nil
	case TypeBool, TypeBool8:
		return d.decodeBits(wordOffset, 8)
	case TypeBool16:
		return d.decodeBits(wordOffset, 16)
	}
	if t.IsString() {
		n, err := t.StringByteLen()
		if err != nil {
			return nil, err
		}
		width := (n + 1) / 2
		b, err := d.extract(wordOffset, width)
		if err != nil {
			return nil, err
		}
		// Trim trailing NULs to match modpoll's Python behaviour.
		return strings.TrimRight(string(b[:n]), "\x00"), nil
	}
	return nil, fmt.Errorf("unsupported data type %q", t)
}

// DecodeBit reads a 16-bit register at wordOffset (with the configured byte
// order applied) and extracts a single bit (0..15, LSB-first).
func (d *RegisterDecoder) DecodeBit(wordOffset, bit int) (bool, error) {
	if bit < 0 || bit > 15 {
		return false, fmt.Errorf("bit %d out of range 0..15", bit)
	}
	b, err := d.extract(wordOffset, 1)
	if err != nil {
		return false, err
	}
	v := binary.BigEndian.Uint16(b)
	return (v>>uint(bit))&1 == 1, nil
}

func (d *RegisterDecoder) decodeBits(wordOffset, n int) ([]bool, error) {
	bytesNeeded := (n + 7) / 8
	width := (bytesNeeded + 1) / 2
	if width == 0 {
		width = 1
	}
	b, err := d.extract(wordOffset, width)
	if err != nil {
		return nil, err
	}
	out := make([]bool, n)
	for i := 0; i < n; i++ {
		out[i] = (b[i/8]>>uint(7-(i%8)))&1 == 1
	}
	return out, nil
}

// CoilDecoder decodes references against a bit slice returned by FC1/FC2.
// modpoll addresses bool8/bool16 references in byte units relative to the
// poller start address.
type CoilDecoder struct {
	bits []bool
}

// NewCoilDecoder constructs a decoder over a raw bit slice. Only as many bits
// as the poller requested will be present.
func NewCoilDecoder(bits []bool) *CoilDecoder {
	return &CoilDecoder{bits: bits}
}

// DecodeAt decodes bool/bool8/bool16 at the given byte offset (each byte is
// eight coils). Other types are not supported on bitwise function codes.
func (c *CoilDecoder) DecodeAt(byteOffset int, t DataType) (any, error) {
	switch t {
	case TypeBool, TypeBool8:
		return c.slice(byteOffset*8, 8)
	case TypeBool16:
		return c.slice(byteOffset*8, 16)
	}
	return nil, fmt.Errorf("coil decoder cannot decode %q", t)
}

func (c *CoilDecoder) slice(start, n int) ([]bool, error) {
	if start < 0 || start+n > len(c.bits) {
		return nil, fmt.Errorf("bit slice [%d:%d] out of range (have %d bits)", start, start+n, len(c.bits))
	}
	out := make([]bool, n)
	copy(out, c.bits[start:start+n])
	return out, nil
}
