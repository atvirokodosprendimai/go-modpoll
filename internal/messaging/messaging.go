// Package messaging carries polled data and write commands across NATS.
//
// Subjects replace the MQTT topics used in the original Python tool:
//
//   modpoll.<device>.data         — published payloads
//   modpoll.<device>.diagnostics  — per-device polling diagnostics
//   modpoll.*.set                 — subscription wildcard for write commands
//
// Subject patterns may be customized; use the literal `{device}` placeholder
// in publish/diagnostics patterns and the NATS `*` wildcard in subscribe
// patterns.
package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
)

const DevicePlaceholder = "{device}"

// WriteCommand is the JSON payload published on the subscribe subject to
// request a Modbus write.
type WriteCommand struct {
	ObjectType string `json:"object_type"`
	Address    int    `json:"address"`
	// Value may be a number, slice of numbers, or boolean depending on the
	// target register. It is kept as json.RawMessage so callers can decode
	// against the target type.
	Value json.RawMessage `json:"value"`
}

// IncomingCommand pairs a parsed command with the device name extracted from
// the subject. DeviceName comes from the wildcard token in the subscribe
// pattern.
type IncomingCommand struct {
	DeviceName string
	Command    WriteCommand
}

// Config holds NATS connection details.
type Config struct {
	URL                       string
	Name                      string
	User                      string
	Password                  string
	Token                     string
	CredsFile                 string
	TLS                       bool
	PublishSubjectPattern     string
	SubscribeSubjectPattern   string
	DiagnosticsSubjectPattern string
	ReconnectWait             time.Duration
	MaxReconnects             int
}

// Handler owns the live NATS connection plus the inbox channel for write
// commands received on the subscribe subject.
type Handler struct {
	cfg  Config
	log  *slog.Logger
	conn *nats.Conn
	sub  *nats.Subscription
	rx   chan IncomingCommand
}

// New connects to NATS, subscribes to the configured subject, and returns a
// ready-to-use Handler.
func New(cfg Config, log *slog.Logger) (*Handler, error) {
	if cfg.URL == "" {
		cfg.URL = nats.DefaultURL
	}
	if cfg.PublishSubjectPattern == "" {
		cfg.PublishSubjectPattern = "modpoll." + DevicePlaceholder + ".data"
	}
	if cfg.SubscribeSubjectPattern == "" {
		cfg.SubscribeSubjectPattern = "modpoll.*.set"
	}
	if cfg.DiagnosticsSubjectPattern == "" {
		cfg.DiagnosticsSubjectPattern = "modpoll." + DevicePlaceholder + ".diagnostics"
	}
	if cfg.ReconnectWait == 0 {
		cfg.ReconnectWait = 2 * time.Second
	}
	if cfg.MaxReconnects == 0 {
		cfg.MaxReconnects = -1
	}

	opts := []nats.Option{
		nats.Name(orDefault(cfg.Name, "modpoll")),
		nats.ReconnectWait(cfg.ReconnectWait),
		nats.MaxReconnects(cfg.MaxReconnects),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			log.Warn("nats disconnected", "err", err)
		}),
		nats.ReconnectHandler(func(c *nats.Conn) {
			log.Info("nats reconnected", "url", c.ConnectedUrl())
		}),
	}
	switch {
	case cfg.CredsFile != "":
		opts = append(opts, nats.UserCredentials(cfg.CredsFile))
	case cfg.Token != "":
		opts = append(opts, nats.Token(cfg.Token))
	case cfg.User != "":
		opts = append(opts, nats.UserInfo(cfg.User, cfg.Password))
	}
	if cfg.TLS {
		opts = append(opts, nats.Secure(nil))
	}

	nc, err := nats.Connect(cfg.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("connect nats: %w", err)
	}
	h := &Handler{cfg: cfg, log: log, conn: nc, rx: make(chan IncomingCommand, 256)}
	if err := h.subscribe(); err != nil {
		nc.Close()
		return nil, err
	}
	log.Info("nats connected", "url", nc.ConnectedUrl(), "subscribe", cfg.SubscribeSubjectPattern)
	return h, nil
}

func (h *Handler) subscribe() error {
	sub, err := h.conn.Subscribe(h.cfg.SubscribeSubjectPattern, h.handleMessage)
	if err != nil {
		return fmt.Errorf("subscribe %s: %w", h.cfg.SubscribeSubjectPattern, err)
	}
	h.sub = sub
	return nil
}

func (h *Handler) handleMessage(m *nats.Msg) {
	device := extractDevice(h.cfg.SubscribeSubjectPattern, m.Subject)
	if device == "" {
		h.log.Warn("could not extract device from subject", "subject", m.Subject)
		return
	}
	var cmd WriteCommand
	if err := json.Unmarshal(m.Data, &cmd); err != nil {
		h.log.Warn("invalid write command payload", "err", err, "subject", m.Subject)
		return
	}
	select {
	case h.rx <- IncomingCommand{DeviceName: device, Command: cmd}:
	default:
		h.log.Warn("write command queue full, dropping message", "subject", m.Subject)
	}
}

// Commands returns a channel that emits incoming write commands.
func (h *Handler) Commands() <-chan IncomingCommand {
	return h.rx
}

// Publish encodes payload as JSON and sends it on the configured data subject
// for the named device.
func (h *Handler) Publish(device string, payload any) error {
	return h.publish(h.cfg.PublishSubjectPattern, device, payload)
}

// PublishDiagnostics publishes diagnostic counters for the named device.
func (h *Handler) PublishDiagnostics(device string, payload any) error {
	return h.publish(h.cfg.DiagnosticsSubjectPattern, device, payload)
}

// PublishSingle publishes one register's value to a child subject (used when
// the --nats-single mode is enabled).
func (h *Handler) PublishSingle(device, refName string, payload any) error {
	base := strings.ReplaceAll(h.cfg.PublishSubjectPattern, DevicePlaceholder, device)
	subject := base + "." + sanitizeSubjectToken(refName)
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return h.conn.Publish(subject, data)
}

func (h *Handler) publish(pattern, device string, payload any) error {
	subject := strings.ReplaceAll(pattern, DevicePlaceholder, device)
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return h.conn.Publish(subject, data)
}

// Drain stops the subscription and closes the underlying connection.
func (h *Handler) Drain(ctx context.Context) {
	if h.sub != nil {
		_ = h.sub.Unsubscribe()
	}
	if h.conn != nil {
		// Drain so in-flight publishes flush; bound by ctx timeout.
		drainDone := make(chan struct{})
		go func() {
			_ = h.conn.Drain()
			close(drainDone)
		}()
		select {
		case <-drainDone:
		case <-ctx.Done():
			h.conn.Close()
		}
	}
}

// extractDevice walks the subscribe subject pattern and the actual subject in
// parallel, returning the token that occupies the position of the first `*`
// wildcard. Literal tokens must match exactly. Multi-token wildcards (`>`) are
// not supported as device markers.
func extractDevice(pattern, subject string) string {
	pTokens := strings.Split(pattern, ".")
	sTokens := strings.Split(subject, ".")
	if len(pTokens) != len(sTokens) {
		return ""
	}
	device := ""
	for i, p := range pTokens {
		if p == "*" {
			if device == "" {
				device = sTokens[i]
			}
			continue
		}
		if p != sTokens[i] {
			return ""
		}
	}
	return device
}

// sanitizeSubjectToken replaces characters that NATS treats as delimiters with
// underscores so reference names can be embedded safely.
func sanitizeSubjectToken(s string) string {
	r := strings.NewReplacer(".", "_", "*", "_", ">", "_", " ", "_")
	return r.Replace(s)
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
