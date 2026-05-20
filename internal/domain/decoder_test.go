package domain

import "testing"

func TestRegisterDecoder_BasicTypes(t *testing.T) {
	d := NewRegisterDecoder([]uint16{0x1234, 0xFFFF, 0x0001, 0x0000}, DefaultEndian())

	got, err := d.DecodeAt(0, TypeUint16)
	if err != nil {
		t.Fatal(err)
	}
	if got.(uint16) != 0x1234 {
		t.Errorf("uint16: got %#x want %#x", got, 0x1234)
	}

	got, err = d.DecodeAt(1, TypeInt16)
	if err != nil {
		t.Fatal(err)
	}
	if got.(int16) != -1 {
		t.Errorf("int16: got %d want -1", got)
	}

	got, err = d.DecodeAt(2, TypeUint32)
	if err != nil {
		t.Fatal(err)
	}
	if got.(uint32) != 0x00010000 {
		t.Errorf("uint32: got %#x want %#x", got, 0x00010000)
	}
}

func TestRegisterDecoder_LittleEndianBytes(t *testing.T) {
	d := NewRegisterDecoder([]uint16{0x8001}, Endian{Bytes: LittleEndian, Words: BigEndian})

	got, err := d.DecodeAt(0, TypeUint16)
	if err != nil {
		t.Fatal(err)
	}
	if got.(uint16) != 0x0180 {
		t.Errorf("LE byte order: got %#x want %#x", got, 0x0180)
	}
}

func TestRegisterDecoder_BitExtraction(t *testing.T) {
	d := NewRegisterDecoder([]uint16{0x8001}, Endian{Bytes: LittleEndian, Words: BigEndian})

	// After LE byte swap: 0x0180. Bit 15 clear, bit 7 set.
	bit15, err := d.DecodeBit(0, 15)
	if err != nil {
		t.Fatal(err)
	}
	if bit15 {
		t.Errorf("bit15: got true want false")
	}

	bit7, err := d.DecodeBit(0, 7)
	if err != nil {
		t.Fatal(err)
	}
	if !bit7 {
		t.Errorf("bit7: got false want true")
	}
}

func TestRegisterDecoder_WordOrderLE(t *testing.T) {
	// 32-bit value 0x11223344 with BE words: registers = [0x1122, 0x3344].
	// With LE word order the wire would carry [0x3344, 0x1122] and we want
	// to recover 0x11223344.
	d := NewRegisterDecoder([]uint16{0x3344, 0x1122}, Endian{Bytes: BigEndian, Words: LittleEndian})

	got, err := d.DecodeAt(0, TypeUint32)
	if err != nil {
		t.Fatal(err)
	}
	if got.(uint32) != 0x11223344 {
		t.Errorf("uint32 LE words: got %#x want %#x", got, 0x11223344)
	}
}

func TestRegisterDecoder_String(t *testing.T) {
	// "Hi" = 0x48 0x69 packed into one register (0x4869), zero-padded.
	d := NewRegisterDecoder([]uint16{0x4869, 0x0000}, DefaultEndian())
	got, err := d.DecodeAt(0, DataType("string4"))
	if err != nil {
		t.Fatal(err)
	}
	if got.(string) != "Hi" {
		t.Errorf("string: got %q want %q", got, "Hi")
	}
}

func TestCoilDecoder_Bool8(t *testing.T) {
	bits := []bool{true, false, true, false, true, false, true, false,
		false, true, false, true, false, true, false, true}
	c := NewCoilDecoder(bits)

	first, err := c.DecodeAt(0, TypeBool8)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range bits[:8] {
		if first.([]bool)[i] != want {
			t.Errorf("first[%d]: got %v want %v", i, first.([]bool)[i], want)
		}
	}

	second, err := c.DecodeAt(1, TypeBool8)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range bits[8:] {
		if second.([]bool)[i] != want {
			t.Errorf("second[%d]: got %v want %v", i, second.([]bool)[i], want)
		}
	}
}

func TestReference_BitSyntaxRequiresBool(t *testing.T) {
	if _, err := NewReference("ok", "40000:5", TypeBool, "r", "", 0); err != nil {
		t.Errorf("bool+bit should be valid: %v", err)
	}
	if _, err := NewReference("bad", "40000:5", TypeUint16, "r", "", 0); err == nil {
		t.Errorf("uint16+bit should be rejected")
	}
	if _, err := NewReference("bad", "40000:5", TypeInt32, "r", "", 0); err == nil {
		t.Errorf("int32+bit should be rejected")
	}
}

func TestReference_InRange(t *testing.T) {
	r, err := NewReference("r1", "40005", TypeUint32, "r", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !r.InRange(40000, 10) {
		t.Errorf("expected in range")
	}
	if r.InRange(40000, 6) {
		t.Errorf("expected out of range (width spills past size)")
	}
}

func TestReference_ScaleAppliedToNumerics(t *testing.T) {
	r, err := NewReference("r1", "40000", TypeInt32, "r", "", 0.001)
	if err != nil {
		t.Fatal(err)
	}
	r.UpdateValue(int32(1500))
	if got, ok := r.Val.(float64); !ok || got != 1.5 {
		t.Errorf("scale: got %v want 1.5", r.Val)
	}
}
