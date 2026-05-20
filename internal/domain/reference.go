package domain

import (
	"fmt"
	"strconv"
	"strings"
)

// Reference describes a single decoded value within a Poller's address range.
//
// A Reference is a pure value object: it carries metadata (name, address,
// type, unit, scale) plus the last decoded value. The polling service is in
// charge of mutating Val and LastVal.
type Reference struct {
	Name    string
	Address int
	Bit     int  // -1 when not a bit reference.
	HasBit  bool
	Type    DataType
	RW      string
	Unit    string
	Scale   float64

	Val     any
	LastVal any
}

// NewReference validates the inputs and constructs a Reference. The address
// string supports the form "<addr>" or "<addr>:<bit>" where bit is 0..15.
func NewReference(name, address string, dtype DataType, rw, unit string, scale float64) (*Reference, error) {
	addr, bit, hasBit, err := parseAddress(address)
	if err != nil {
		return nil, fmt.Errorf("reference %q: %w", name, err)
	}
	if hasBit && dtype != TypeBool {
		return nil, fmt.Errorf("reference %q: bit syntax can only be used with dtype 'bool', got %q", name, dtype)
	}
	if rw == "" {
		rw = "r"
	}
	return &Reference{
		Name:    strings.ReplaceAll(name, " ", "_"),
		Address: addr,
		Bit:     bit,
		HasBit:  hasBit,
		Type:    dtype,
		RW:      strings.ToLower(rw),
		Unit:    unit,
		Scale:   scale,
	}, nil
}

// Width is the address span this reference occupies (in registers for FC 3/4
// or in 8-bit bytes for FC 1/2).
func (r *Reference) Width() int {
	if r.HasBit {
		return 1
	}
	return r.Type.Width()
}

// Readable reports whether the reference should be polled.
func (r *Reference) Readable() bool {
	return strings.Contains(r.RW, "r")
}

// Writable reports whether the reference may be written to.
func (r *Reference) Writable() bool {
	return strings.Contains(r.RW, "w")
}

// InRange returns true when the reference fully fits within [start, start+size).
func (r *Reference) InRange(start, size int) bool {
	end := start + size
	last := r.Address + r.Width() - 1
	return r.Address >= start && r.Address < end && last >= start && last < end
}

// UpdateValue records the decoded value, applying scale to scalar numbers.
func (r *Reference) UpdateValue(v any) {
	r.LastVal = r.Val
	r.Val = applyScale(v, r.Scale)
}

func applyScale(v any, scale float64) any {
	if scale == 0 || scale == 1 {
		return v
	}
	switch x := v.(type) {
	case int8:
		return float64(x) * scale
	case int16:
		return float64(x) * scale
	case int32:
		return float64(x) * scale
	case int64:
		return float64(x) * scale
	case uint8:
		return float64(x) * scale
	case uint16:
		return float64(x) * scale
	case uint32:
		return float64(x) * scale
	case uint64:
		return float64(x) * scale
	case float32:
		return float64(x) * scale
	case float64:
		return x * scale
	}
	return v
}

func parseAddress(s string) (addr, bit int, hasBit bool, err error) {
	s = strings.TrimSpace(s)
	if idx := strings.Index(s, ":"); idx >= 0 {
		a, parseErr := parseInt(s[:idx])
		if parseErr != nil {
			return 0, 0, false, fmt.Errorf("invalid address: %w", parseErr)
		}
		b, parseErr := strconv.Atoi(s[idx+1:])
		if parseErr != nil {
			return 0, 0, false, fmt.Errorf("invalid bit index: %w", parseErr)
		}
		if b < 0 || b > 15 {
			return 0, 0, false, fmt.Errorf("bit index must be 0..15, got %d", b)
		}
		return a, b, true, nil
	}
	a, parseErr := parseInt(s)
	if parseErr != nil {
		return 0, 0, false, fmt.Errorf("invalid address: %w", parseErr)
	}
	return a, -1, false, nil
}

// parseInt parses a base-aware integer (0x..., 0b..., decimal).
func parseInt(s string) (int, error) {
	s = strings.TrimSpace(s)
	v, err := strconv.ParseInt(s, 0, 64)
	if err != nil {
		return 0, err
	}
	return int(v), nil
}
