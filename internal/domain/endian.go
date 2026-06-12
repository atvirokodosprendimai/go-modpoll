package domain

import "strings"

// ByteOrder represents a byte/word ordering choice.
type ByteOrder uint8

const (
	BigEndian ByteOrder = iota
	LittleEndian
)

// Endian captures both the byte ordering within each 16-bit register and the
// ordering of consecutive registers when assembled into multi-word values.
type Endian struct {
	Bytes ByteOrder
	Words ByteOrder
}

// DefaultEndian returns BE_BE which is the most common Modbus encoding.
func DefaultEndian() Endian {
	return Endian{Bytes: BigEndian, Words: BigEndian}
}

// ParseEndian parses strings like "BE_BE", "LE_BE", "LE_LE", "BE_LE", plus
// the short forms "BE" and "LE". Empty strings and unknown values fall back
// to BE_LE to match modpoll's Python behaviour, where pymodbus defaulted to
// byteorder=BIG and wordorder=LITTLE for any unspecified endian field.
//
// Short forms set only the byte order and leave the word order at the Python
// default (LITTLE):
//
//	"BE" → Endian{Bytes: BigEndian,    Words: LittleEndian}
//	"LE" → Endian{Bytes: LittleEndian, Words: LittleEndian}
func ParseEndian(s string) (Endian, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	switch s {
	case "BE_BE":
		return Endian{Bytes: BigEndian, Words: BigEndian}, nil
	case "LE_BE":
		return Endian{Bytes: LittleEndian, Words: BigEndian}, nil
	case "LE_LE":
		return Endian{Bytes: LittleEndian, Words: LittleEndian}, nil
	case "BE_LE", "BE", "":
		return Endian{Bytes: BigEndian, Words: LittleEndian}, nil
	case "LE":
		return Endian{Bytes: LittleEndian, Words: LittleEndian}, nil
	}
	// Unknown strings: match Python's permissive fallback to BE_LE.
	return Endian{Bytes: BigEndian, Words: LittleEndian}, nil
}
