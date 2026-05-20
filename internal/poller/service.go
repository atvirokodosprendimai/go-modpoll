// Package poller orchestrates polling cycles against a Modbus client and
// publishes results through any registered publisher.
package poller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/atvirokodosprendimai/go-modpoll/internal/domain"
	"github.com/atvirokodosprendimai/go-modpoll/internal/messaging"
	"github.com/atvirokodosprendimai/go-modpoll/internal/modbus"
)

// Publisher abstracts the messaging layer; the service uses it without
// caring whether it talks to NATS or anything else.
type Publisher interface {
	Publish(device string, payload any) error
	PublishDiagnostics(device string, payload any) error
	PublishSingle(device, refName string, payload any) error
}

// Options tune the polling loop's behaviour.
type Options struct {
	Interval       time.Duration
	WithTimestamp  bool
	SinglePublish  bool
	Daemon         bool
	AutoRemove     bool
	AutoRemoveAt   int // failures before disabling a poller
	FloatPrecision int
}

// DefaultOptions provides sane defaults that mirror the Python tool.
func DefaultOptions() Options {
	return Options{
		Interval:       500 * time.Millisecond,
		AutoRemoveAt:   3,
		FloatPrecision: 3,
	}
}

// Service binds a Modbus client to one or more device aggregates and a
// publisher.
type Service struct {
	client  modbus.Client
	devices []*domain.Device
	pub     Publisher
	opts    Options
	log     *slog.Logger
}

// New constructs a Service. The Modbus client is opened lazily on the first
// poll so that the loop can survive transient connection failures.
func New(client modbus.Client, devices []*domain.Device, pub Publisher, opts Options, log *slog.Logger) *Service {
	if opts.FloatPrecision <= 0 {
		opts.FloatPrecision = 3
	}
	return &Service{client: client, devices: devices, pub: pub, opts: opts, log: log}
}

// Devices exposes the configured device aggregates.
func (s *Service) Devices() []*domain.Device { return s.devices }

// PollAll iterates all configured pollers, decoding responses into references.
// It returns the first context cancellation it observes.
func (s *Service) PollAll(ctx context.Context) error {
	if err := s.client.Open(); err != nil {
		s.log.Error("modbus open failed", "err", err)
		return err
	}
	defer s.client.Close()

	for _, dev := range s.devices {
		for _, p := range dev.Pollers {
			if p.Disabled {
				continue
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			ok := s.pollOne(dev, p)
			dev.RecordResult(ok)
			if !ok && s.opts.AutoRemove && p.FailCount >= s.opts.AutoRemoveAt {
				s.log.Warn("disabling poller after repeated failures",
					"device", dev.Name, "fc", p.FC, "addr", p.StartAddress)
				p.Disabled = true
			}
			if s.opts.Interval > 0 {
				select {
				case <-time.After(s.opts.Interval):
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
	}
	return nil
}

func (s *Service) pollOne(dev *domain.Device, p *domain.Poller) bool {
	switch p.FC {
	case domain.FCCoil, domain.FCDiscreteInput:
		bits, err := s.readBits(dev, p)
		if err != nil {
			return s.fail(dev, p, err)
		}
		dec := domain.NewCoilDecoder(bits)
		for _, ref := range p.ReadableRefs {
			byteOffset := ref.Address - p.StartAddress
			v, err := dec.DecodeAt(byteOffset, ref.Type)
			if err != nil {
				s.log.Warn("coil decode failed", "ref", ref.Name, "err", err)
				ref.UpdateValue(nil)
				continue
			}
			ref.UpdateValue(v)
		}
		p.FailCount = 0
		return true
	case domain.FCHoldingRegister, domain.FCInputRegister:
		regs, err := s.readRegisters(dev, p)
		if err != nil {
			return s.fail(dev, p, err)
		}
		dec := domain.NewRegisterDecoder(regs, p.Endian)
		for _, ref := range p.ReadableRefs {
			wordOffset := ref.Address - p.StartAddress
			var v any
			if ref.HasBit {
				bit, err := dec.DecodeBit(wordOffset, ref.Bit)
				if err != nil {
					s.log.Warn("bit decode failed", "ref", ref.Name, "err", err)
					ref.UpdateValue(nil)
					continue
				}
				v = bit
			} else {
				dv, err := dec.DecodeAt(wordOffset, ref.Type)
				if err != nil {
					s.log.Warn("register decode failed", "ref", ref.Name, "err", err)
					ref.UpdateValue(nil)
					continue
				}
				v = dv
			}
			ref.UpdateValue(v)
		}
		p.FailCount = 0
		return true
	}
	return false
}

func (s *Service) readBits(dev *domain.Device, p *domain.Poller) ([]bool, error) {
	addr := uint16(p.StartAddress)
	qty := uint16(p.Size)
	switch p.FC {
	case domain.FCCoil:
		return s.client.ReadCoils(dev.ID, addr, qty)
	case domain.FCDiscreteInput:
		return s.client.ReadDiscreteInputs(dev.ID, addr, qty)
	}
	return nil, fmt.Errorf("unexpected fc %v", p.FC)
}

func (s *Service) readRegisters(dev *domain.Device, p *domain.Poller) ([]uint16, error) {
	addr := uint16(p.StartAddress)
	qty := uint16(p.Size)
	switch p.FC {
	case domain.FCHoldingRegister:
		return s.client.ReadHoldingRegisters(dev.ID, addr, qty)
	case domain.FCInputRegister:
		return s.client.ReadInputRegisters(dev.ID, addr, qty)
	}
	return nil, fmt.Errorf("unexpected fc %v", p.FC)
}

func (s *Service) fail(dev *domain.Device, p *domain.Poller, err error) bool {
	p.FailCount++
	for _, ref := range p.ReadableRefs {
		ref.UpdateValue(nil)
	}
	s.log.Error("modbus poll failed",
		"device", dev.Name, "fc", p.FC, "addr", p.StartAddress, "err", err)
	return false
}

// PublishData publishes the most recently decoded values for every device.
// onChange skips references whose value has not changed since the previous
// poll.
func (s *Service) PublishData(timestamp time.Time, onChange bool) {
	if s.pub == nil {
		return
	}
	for _, dev := range s.devices {
		if !dev.LastPollOK {
			s.log.Debug("skip publish for disconnected device", "device", dev.Name)
			continue
		}
		payload := map[string]any{}
		for _, ref := range dev.References {
			if onChange && fmt.Sprintf("%v", ref.Val) == fmt.Sprintf("%v", ref.LastVal) {
				continue
			}
			val := roundIfFloat(ref.Val, s.opts.FloatPrecision)
			key := ref.Name
			if ref.Unit != "" {
				key = fmt.Sprintf("%s|%s", ref.Name, ref.Unit)
			}
			payload[key] = val
			if s.opts.SinglePublish {
				if err := publishSingle(s.pub, dev.Name, ref.Name, val); err != nil {
					s.log.Warn("single publish failed", "ref", ref.Name, "err", err)
				}
			}
		}
		if len(payload) == 0 || s.opts.SinglePublish {
			continue
		}
		if s.opts.WithTimestamp {
			payload["timestamp"] = timestamp.UTC().Format(time.RFC3339Nano)
		}
		if err := s.pub.Publish(dev.Name, payload); err != nil {
			s.log.Warn("publish failed", "device", dev.Name, "err", err)
		}
	}
}

// PublishDiagnostics emits per-device counters on the diagnostics subject.
func (s *Service) PublishDiagnostics() {
	if s.pub == nil {
		return
	}
	for _, dev := range s.devices {
		payload := map[string]any{
			"poll_count":        dev.PollCount,
			"error_count":       dev.ErrorCount,
			"last_poll_success": dev.LastPollOK,
		}
		if err := s.pub.PublishDiagnostics(dev.Name, payload); err != nil {
			s.log.Warn("publish diagnostics failed", "device", dev.Name, "err", err)
		}
	}
}

func publishSingle(p Publisher, device, ref string, val any) error {
	switch v := val.(type) {
	case []bool:
		for i, item := range v {
			child := fmt.Sprintf("%s_%d", ref, i)
			if err := p.PublishSingle(device, child, item); err != nil {
				return err
			}
		}
		return nil
	default:
		return p.PublishSingle(device, ref, val)
	}
}

// ApplyCommand interprets a NATS write command and dispatches the matching
// Modbus write.
func (s *Service) ApplyCommand(cmd messaging.IncomingCommand) error {
	dev := s.findDevice(cmd.DeviceName)
	if dev == nil {
		return fmt.Errorf("device %q not configured", cmd.DeviceName)
	}
	if err := s.client.Open(); err != nil {
		return fmt.Errorf("open modbus: %w", err)
	}
	defer s.client.Close()

	switch strings.ToLower(cmd.Command.ObjectType) {
	case "coil":
		var v bool
		if err := json.Unmarshal(cmd.Command.Value, &v); err != nil {
			return fmt.Errorf("decode coil value: %w", err)
		}
		return s.client.WriteSingleCoil(dev.ID, uint16(cmd.Command.Address), v)
	case "holding_register":
		// Try single uint16 first, then []uint16.
		var single uint16
		if err := json.Unmarshal(cmd.Command.Value, &single); err == nil {
			return s.client.WriteSingleRegister(dev.ID, uint16(cmd.Command.Address), single)
		}
		var many []uint16
		if err := json.Unmarshal(cmd.Command.Value, &many); err != nil {
			return fmt.Errorf("decode register value: %w", err)
		}
		return s.client.WriteMultipleRegisters(dev.ID, uint16(cmd.Command.Address), many)
	}
	return fmt.Errorf("unsupported object_type %q", cmd.Command.ObjectType)
}

func (s *Service) findDevice(name string) *domain.Device {
	for _, dev := range s.devices {
		if dev.Name == name {
			return dev
		}
	}
	return nil
}

// PrintResults prints decoded references for every device as a simple table.
func (s *Service) PrintResults() {
	for _, dev := range s.devices {
		fmt.Printf("\nDevice: %s\n", dev.Name)
		PrintTable(dev, s.opts.FloatPrecision)
	}
}

func roundIfFloat(v any, precision int) any {
	switch x := v.(type) {
	case float32:
		return round(float64(x), precision)
	case float64:
		return round(x, precision)
	}
	return v
}

func round(v float64, precision int) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return v
	}
	pow := math.Pow(10, float64(precision))
	return math.Round(v*pow) / pow
}

// PublishOrErr is a convenience used by main.go to surface fatal publish
// failures during the initial diagnostics burst.
var ErrNoPublisher = errors.New("no publisher configured")
