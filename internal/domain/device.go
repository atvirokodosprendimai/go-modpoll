package domain

// Device is the root aggregate. It owns a list of Pollers and the union of
// References reachable through them.
type Device struct {
	Name string
	ID   uint8

	Pollers    []*Poller
	References map[string]*Reference

	PollCount    int
	ErrorCount   int
	LastPollOK   bool
}

// NewDevice creates an empty device aggregate.
func NewDevice(name string, id uint8) *Device {
	return &Device{
		Name:       name,
		ID:         id,
		References: make(map[string]*Reference),
	}
}

// AddReference associates a reference with the device's name lookup table.
func (d *Device) AddReference(r *Reference) {
	d.References[r.Name] = r
}

// RecordResult updates the diagnostics counters after a polling attempt.
func (d *Device) RecordResult(ok bool) {
	d.PollCount++
	if ok {
		d.LastPollOK = true
		return
	}
	d.LastPollOK = false
	d.ErrorCount++
}
