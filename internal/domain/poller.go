package domain

import "fmt"

// FunctionCode enumerates the Modbus function codes used for polling.
type FunctionCode uint8

const (
	FCCoil            FunctionCode = 1
	FCDiscreteInput   FunctionCode = 2
	FCHoldingRegister FunctionCode = 3
	FCInputRegister   FunctionCode = 4
)

// ParseObjectType maps modpoll's CSV labels to Modbus function codes.
func ParseObjectType(label string) (FunctionCode, error) {
	switch label {
	case "coil":
		return FCCoil, nil
	case "discrete_input":
		return FCDiscreteInput, nil
	case "holding_register":
		return FCHoldingRegister, nil
	case "input_register":
		return FCInputRegister, nil
	}
	return 0, fmt.Errorf("unknown object type %q", label)
}

// IsBitwise reports whether the function code addresses individual bits (coils
// and discrete inputs) as opposed to 16-bit registers.
func (f FunctionCode) IsBitwise() bool {
	return f == FCCoil || f == FCDiscreteInput
}

// Poller is a contiguous range of registers (or coils) read in a single
// Modbus request. References attached to a Poller are decoded from the
// response payload.
type Poller struct {
	FC           FunctionCode
	StartAddress int
	Size         int
	Endian       Endian

	ReadableRefs []*Reference

	// Disabled is set by the polling service when autoremove triggers.
	Disabled bool
	// FailCount tracks consecutive read failures.
	FailCount int
}

// NewPoller validates basic Modbus size limits and constructs the poller.
func NewPoller(fc FunctionCode, start, size int, endian Endian) (*Poller, error) {
	if fc.IsBitwise() && size > 2000 {
		return nil, fmt.Errorf("too many coils/discrete inputs (max 2000): %d", size)
	}
	if !fc.IsBitwise() && size > 123 {
		return nil, fmt.Errorf("too many registers (max 123): %d", size)
	}
	return &Poller{FC: fc, StartAddress: start, Size: size, Endian: endian}, nil
}

// AddReadable appends a reference if it is not already present.
func (p *Poller) AddReadable(r *Reference) {
	for _, existing := range p.ReadableRefs {
		if existing.Address == r.Address && existing.Bit == r.Bit {
			return
		}
	}
	p.ReadableRefs = append(p.ReadableRefs, r)
}
