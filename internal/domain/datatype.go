package domain

import (
	"fmt"
	"strconv"
	"strings"
)

// DataType identifies how raw register/coil bytes are decoded into Go values.
type DataType string

const (
	TypeUint16  DataType = "uint16"
	TypeInt16   DataType = "int16"
	TypeUint32  DataType = "uint32"
	TypeInt32   DataType = "int32"
	TypeUint64  DataType = "uint64"
	TypeInt64   DataType = "int64"
	TypeFloat32 DataType = "float32"
	TypeFloat64 DataType = "float64"
	TypeBool    DataType = "bool"
	TypeBool8   DataType = "bool8"
	TypeBool16  DataType = "bool16"
)

// IsString reports whether the type is a string with a fixed byte length, e.g.
// "string16".
func (d DataType) IsString() bool {
	return strings.HasPrefix(string(d), "string")
}

// StringByteLen parses the byte length encoded in a "stringXXX" type.
func (d DataType) StringByteLen() (int, error) {
	if !d.IsString() {
		return 0, fmt.Errorf("not a string type: %q", d)
	}
	n, err := strconv.Atoi(strings.TrimPrefix(string(d), "string"))
	if err != nil {
		return 0, fmt.Errorf("invalid string length in %q: %w", d, err)
	}
	return n, nil
}

// Width returns the number of 16-bit registers occupied by a value of this
// data type. For coil/discrete inputs the same value is interpreted as the
// number of 8-bit groups (bytes).
func (d DataType) Width() int {
	switch d {
	case TypeUint16, TypeInt16, TypeBool, TypeBool8:
		return 1
	case TypeUint32, TypeInt32, TypeFloat32, TypeBool16:
		return 2
	case TypeUint64, TypeInt64, TypeFloat64:
		return 4
	}
	if d.IsString() {
		n, err := d.StringByteLen()
		if err != nil {
			return 1
		}
		return (n + 1) / 2
	}
	return 1
}

// Normalize lower-cases the type so comparisons are case-insensitive.
func Normalize(s string) DataType {
	return DataType(strings.ToLower(strings.TrimSpace(s)))
}
