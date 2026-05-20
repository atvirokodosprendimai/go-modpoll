package config

import (
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
)

func newLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestLoad_ModsimExample(t *testing.T) {
	root, err := filepath.Abs("../../examples/modsim.csv")
	if err != nil {
		t.Fatal(err)
	}
	devs, err := Load(root, 5_000_000_000, newLogger())
	if err != nil {
		t.Skipf("modsim.csv not available: %v", err)
	}
	if len(devs) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devs))
	}
	d := devs[0]
	if d.Name != "modsim01" || d.ID != 1 {
		t.Errorf("device meta: %+v", d)
	}
	if len(d.Pollers) != 4 {
		t.Errorf("pollers: got %d want 4", len(d.Pollers))
	}
}

func TestParseDevice_Validation(t *testing.T) {
	if _, err := parseDevice([]string{"device"}); err == nil {
		t.Error("expected error for short row")
	}
	if _, err := parseDevice([]string{"device", "", "1"}); err == nil {
		t.Error("expected error for empty name")
	}
	if _, err := parseDevice([]string{"device", "d", "256"}); err == nil {
		t.Error("expected error for out-of-range id")
	}
}

func TestParsePoller_KnownObjectTypes(t *testing.T) {
	cases := []string{"coil", "discrete_input", "holding_register", "input_register"}
	for _, c := range cases {
		row := []string{"poll", c, "0", "4", "BE_BE"}
		if _, err := parsePoller(row); err != nil {
			t.Errorf("%s: %v", c, err)
		}
	}
	if _, err := parsePoller([]string{"poll", "weird_object", "0", "4", "BE_BE"}); err == nil {
		t.Error("expected unknown object type to fail")
	}
}

func TestParsePoller_Validation(t *testing.T) {
	if _, err := parsePoller([]string{"poll", "coil"}); err == nil {
		t.Error("expected error for short row")
	}
}

func TestParseReference_Basic(t *testing.T) {
	ref, err := parseReference([]string{"ref", "tag1", "40000", "uint16", "rw", "kWh", "0.001"})
	if err != nil {
		t.Fatal(err)
	}
	if ref.Name != "tag1" {
		t.Errorf("name: got %q", ref.Name)
	}
	if ref.Address != 40000 {
		t.Errorf("address: got %d", ref.Address)
	}
	if ref.Unit != "kWh" {
		t.Errorf("unit: got %q", ref.Unit)
	}
	if ref.Scale != 0.001 {
		t.Errorf("scale: got %v", ref.Scale)
	}
}

func TestParseReference_NameSpaceReplaced(t *testing.T) {
	ref, err := parseReference([]string{"ref", "my tag", "0", "uint16", "r"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ref.Name, "_") {
		t.Errorf("expected space replaced with _, got %q", ref.Name)
	}
}
