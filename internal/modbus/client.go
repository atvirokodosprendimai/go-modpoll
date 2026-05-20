// Package modbus wraps a third-party Modbus master implementation so the rest
// of the application talks to it through a narrow domain-oriented interface.
package modbus

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/simonvetter/modbus"
)

// Transport identifies the physical/transport layer in use.
type Transport int

const (
	TransportTCP Transport = iota
	TransportUDP
	TransportSerial
)

// Framer enumerates Modbus framing options. "default" lets each transport
// pick its natural framer (socket for TCP/UDP, RTU for serial).
type Framer string

const (
	FramerDefault Framer = "default"
	FramerASCII   Framer = "ascii"
	FramerRTU     Framer = "rtu"
	FramerSocket  Framer = "socket"
)

// Config captures everything required to open a Modbus connection.
type Config struct {
	Transport Transport
	Framer    Framer
	Host      string
	Port      int

	SerialPort   string
	SerialBaud   int
	SerialParity string // "none", "odd", "even"

	Timeout time.Duration
}

// Client is the narrow contract the rest of the application uses to read and
// write Modbus data points.
type Client interface {
	Open() error
	Close() error
	ReadCoils(unitID uint8, addr, qty uint16) ([]bool, error)
	ReadDiscreteInputs(unitID uint8, addr, qty uint16) ([]bool, error)
	ReadHoldingRegisters(unitID uint8, addr, qty uint16) ([]uint16, error)
	ReadInputRegisters(unitID uint8, addr, qty uint16) ([]uint16, error)
	WriteSingleCoil(unitID uint8, addr uint16, value bool) error
	WriteSingleRegister(unitID uint8, addr, value uint16) error
	WriteMultipleRegisters(unitID uint8, addr uint16, values []uint16) error
}

// NewClient validates the configuration and constructs a Client backed by
// github.com/simonvetter/modbus.
func NewClient(cfg Config) (Client, error) {
	urlStr, parity, err := buildURL(cfg)
	if err != nil {
		return nil, err
	}
	mc, err := modbus.NewClient(&modbus.ClientConfiguration{
		URL:      urlStr,
		Speed:    uint(cfg.SerialBaud),
		DataBits: 8,
		Parity:   parity,
		StopBits: 1,
		Timeout:  cfg.Timeout,
	})
	if err != nil {
		return nil, fmt.Errorf("create modbus client: %w", err)
	}
	return &simonClient{c: mc}, nil
}

func buildURL(cfg Config) (string, uint, error) {
	switch cfg.Transport {
	case TransportTCP:
		if cfg.Framer != FramerDefault && cfg.Framer != FramerSocket {
			return "", 0, fmt.Errorf("framer %q is invalid for TCP transport", cfg.Framer)
		}
		host := cfg.Host
		if host == "" {
			return "", 0, fmt.Errorf("TCP transport requires a host")
		}
		return fmt.Sprintf("tcp://%s:%d", host, cfg.Port), 0, nil
	case TransportUDP:
		if cfg.Framer != FramerDefault && cfg.Framer != FramerSocket {
			return "", 0, fmt.Errorf("framer %q is invalid for UDP transport", cfg.Framer)
		}
		host := cfg.Host
		if host == "" {
			return "", 0, fmt.Errorf("UDP transport requires a host")
		}
		return fmt.Sprintf("udp://%s:%d", host, cfg.Port), 0, nil
	case TransportSerial:
		if cfg.SerialPort == "" {
			return "", 0, fmt.Errorf("serial transport requires a port or URL")
		}
		framer := cfg.Framer
		if framer == FramerDefault {
			framer = FramerRTU
		}
		if framer != FramerRTU && framer != FramerASCII {
			return "", 0, fmt.Errorf("framer %q is invalid for serial transport", framer)
		}
		scheme := "rtu"
		if framer == FramerASCII {
			scheme = "ascii"
		}
		parity := parityCode(cfg.SerialParity)
		return assembleSerialURL(scheme, cfg.SerialPort), parity, nil
	}
	return "", 0, fmt.Errorf("unknown transport %v", cfg.Transport)
}

func assembleSerialURL(scheme, port string) string {
	if strings.Contains(port, "://") {
		return port
	}
	// simonvetter/modbus accepts "rtu:///dev/ttyUSB0" — three slashes after
	// the scheme make the host empty and the path absolute.
	u := &url.URL{Scheme: scheme, Path: port}
	if !strings.HasPrefix(port, "/") {
		u.Path = "/" + port
	}
	return u.String()
}

func parityCode(p string) uint {
	switch strings.ToLower(p) {
	case "odd":
		return modbus.PARITY_ODD
	case "even":
		return modbus.PARITY_EVEN
	}
	return modbus.PARITY_NONE
}

type simonClient struct {
	c *modbus.ModbusClient
}

func (s *simonClient) Open() error  { return s.c.Open() }
func (s *simonClient) Close() error { return s.c.Close() }

func (s *simonClient) setUnit(unitID uint8) error {
	return s.c.SetUnitId(unitID)
}

func (s *simonClient) ReadCoils(unitID uint8, addr, qty uint16) ([]bool, error) {
	if err := s.setUnit(unitID); err != nil {
		return nil, err
	}
	return s.c.ReadCoils(addr, qty)
}

func (s *simonClient) ReadDiscreteInputs(unitID uint8, addr, qty uint16) ([]bool, error) {
	if err := s.setUnit(unitID); err != nil {
		return nil, err
	}
	return s.c.ReadDiscreteInputs(addr, qty)
}

func (s *simonClient) ReadHoldingRegisters(unitID uint8, addr, qty uint16) ([]uint16, error) {
	if err := s.setUnit(unitID); err != nil {
		return nil, err
	}
	return s.c.ReadRegisters(addr, qty, modbus.HOLDING_REGISTER)
}

func (s *simonClient) ReadInputRegisters(unitID uint8, addr, qty uint16) ([]uint16, error) {
	if err := s.setUnit(unitID); err != nil {
		return nil, err
	}
	return s.c.ReadRegisters(addr, qty, modbus.INPUT_REGISTER)
}

func (s *simonClient) WriteSingleCoil(unitID uint8, addr uint16, value bool) error {
	if err := s.setUnit(unitID); err != nil {
		return err
	}
	return s.c.WriteCoil(addr, value)
}

func (s *simonClient) WriteSingleRegister(unitID uint8, addr, value uint16) error {
	if err := s.setUnit(unitID); err != nil {
		return err
	}
	return s.c.WriteRegister(addr, value)
}

func (s *simonClient) WriteMultipleRegisters(unitID uint8, addr uint16, values []uint16) error {
	if err := s.setUnit(unitID); err != nil {
		return err
	}
	return s.c.WriteRegisters(addr, values)
}
