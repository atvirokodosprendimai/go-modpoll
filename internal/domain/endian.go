package domain

import (
	"fmt"
	"strings"
)

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

// ParseEndian parses strings like "BE_BE", "LE_BE", "LE_LE", "BE_LE". Empty
// strings and unknown values fall back to BE_LE to match modpoll's Python
// behaviour where unrecognised endian strings produced byteorder BIG and
// wordorder LITTLE.
func ParseEndian(s string) (Endian, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	switch s {
	case "", "BE_BE":
		return Endian{Bytes: BigEndian, Words: BigEndian}, nil
	case "LE_BE":
		return Endian{Bytes: LittleEndian, Words: BigEndian}, nil
	case "LE_LE":
		return Endian{Bytes: LittleEndian, Words: LittleEndian}, nil
	case "BE_LE":
		return Endian{Bytes: BigEndian, Words: LittleEndian}, nil
	}
	return Endian{}, fmt.Errorf("unknown endian %q", s)
}
