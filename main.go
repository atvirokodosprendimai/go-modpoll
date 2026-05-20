// modpoll command — a Modbus polling tool that publishes data to NATS.
//
// This is a Go rewrite of the original Python tool. Compared to the Python
// version, the messaging layer is NATS (subjects) instead of MQTT (topics).
// All references to MQTT in the Python CLI are replaced with --nats-* flags.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/atvirokodosprendimai/go-modpoll/internal/config"
	"github.com/atvirokodosprendimai/go-modpoll/internal/domain"
	"github.com/atvirokodosprendimai/go-modpoll/internal/exporter"
	"github.com/atvirokodosprendimai/go-modpoll/internal/messaging"
	"github.com/atvirokodosprendimai/go-modpoll/internal/modbus"
	"github.com/atvirokodosprendimai/go-modpoll/internal/poller"
)

// Version is bumped each release. Override at build time via -ldflags.
var Version = "0.1.0"

func main() {
	cmd := &cli.Command{
		Name:    "modpoll",
		Usage:   "Modbus polling tool publishing data to NATS",
		Version: Version,
		Flags:   flags(),
		Action:  run,
	}
	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringSliceFlag{Name: "config", Aliases: []string{"f"}, Required: true,
			Usage: "Local path or URL of Modbus configuration file (repeatable)"},
		&cli.BoolFlag{Name: "daemon", Aliases: []string{"d"},
			Usage: "Run without printing per-poll result tables"},
		&cli.FloatFlag{Name: "rate", Aliases: []string{"r"}, Value: 10.0,
			Usage: "Sampling rate in seconds"},
		&cli.BoolFlag{Name: "once", Aliases: []string{"1"},
			Usage: "Poll once and exit"},
		&cli.FloatFlag{Name: "interval", Value: 0.5,
			Usage: "Seconds between two pollers"},
		&cli.StringFlag{Name: "tcp", Usage: "Modbus TCP host"},
		&cli.IntFlag{Name: "tcp-port", Value: 502, Usage: "Modbus TCP port"},
		&cli.StringFlag{Name: "udp", Usage: "Modbus UDP host"},
		&cli.IntFlag{Name: "udp-port", Value: 502, Usage: "Modbus UDP port"},
		&cli.StringFlag{Name: "serial", Aliases: []string{"rtu"},
			Usage: "Serial device (e.g. /dev/ttyUSB0) or URL"},
		&cli.IntFlag{Name: "serial-baud", Aliases: []string{"rtu-baud"}, Value: 9600,
			Usage: "Serial baud rate"},
		&cli.StringFlag{Name: "serial-parity", Aliases: []string{"rtu-parity"}, Value: "none",
			Usage: "Serial parity: none|odd|even"},
		&cli.FloatFlag{Name: "timeout", Value: 3.0,
			Usage: "Modbus response timeout in seconds"},
		&cli.StringFlag{Name: "export", Aliases: []string{"o"},
			Usage: "Export decoded data to this JSON file each cycle"},
		&cli.StringFlag{Name: "export-http",
			Usage: "POST decoded data as JSON to this URL each cycle"},
		&cli.FloatFlag{Name: "export-http-timeout", Value: 10.0,
			Usage: "Timeout for --export-http POST in seconds"},
		&cli.FloatFlag{Name: "diagnostics-rate", Value: 0,
			Usage: "Seconds between diagnostics publishes (0 disables)"},
		&cli.BoolFlag{Name: "autoremove",
			Usage: "Disable a poller after 3 consecutive failures"},
		&cli.StringFlag{Name: "loglevel", Value: "INFO",
			Usage: "DEBUG|INFO|WARN|ERROR"},
		&cli.BoolFlag{Name: "timestamp",
			Usage: "Add a timestamp to each published payload"},
		&cli.IntFlag{Name: "delay", Value: 0,
			Usage: "Seconds to wait before the first poll"},
		&cli.StringFlag{Name: "framer", Value: "default",
			Usage: "Modbus framer: default|ascii|rtu|socket"},

		&cli.StringFlag{Name: "nats-url", Sources: cli.EnvVars("NATS_URL"),
			Usage: "NATS connection URL (set to enable publishing)"},
		&cli.StringFlag{Name: "nats-name", Value: "modpoll",
			Usage: "Client identifier reported to the NATS server"},
		&cli.StringFlag{Name: "nats-user", Usage: "NATS username"},
		&cli.StringFlag{Name: "nats-pass", Usage: "NATS password"},
		&cli.StringFlag{Name: "nats-token", Usage: "NATS auth token"},
		&cli.StringFlag{Name: "nats-creds", Usage: "Path to a NATS credentials file"},
		&cli.BoolFlag{Name: "nats-tls", Usage: "Use TLS when connecting to NATS"},
		&cli.StringFlag{Name: "nats-publish-subject-pattern",
			Value: "modpoll." + messaging.DevicePlaceholder + ".data",
			Usage: "Subject pattern for published data; {device} is replaced"},
		&cli.StringFlag{Name: "nats-subscribe-subject-pattern",
			Value: "modpoll.*.set",
			Usage: "Wildcard subject pattern for incoming write commands"},
		&cli.StringFlag{Name: "nats-diagnostics-subject-pattern",
			Value: "modpoll." + messaging.DevicePlaceholder + ".diagnostics",
			Usage: "Subject pattern for diagnostics"},
		&cli.BoolFlag{Name: "nats-single",
			Usage: "Publish each reference on its own subject under the device data subject"},
	}
}

func run(ctx context.Context, cmd *cli.Command) error {
	log := newLogger(cmd.String("loglevel"))

	devices, err := loadDevices(cmd, log)
	if err != nil {
		return err
	}
	totalRefs := 0
	for _, d := range devices {
		totalRefs += len(d.References)
	}
	log.Info("loaded devices", "devices", len(devices), "references", totalRefs)

	mb, err := buildModbusClient(cmd)
	if err != nil {
		return err
	}

	var pub *messaging.Handler
	if url := cmd.String("nats-url"); url != "" {
		pub, err = messaging.New(messaging.Config{
			URL:                       url,
			Name:                      cmd.String("nats-name"),
			User:                      cmd.String("nats-user"),
			Password:                  cmd.String("nats-pass"),
			Token:                     cmd.String("nats-token"),
			CredsFile:                 cmd.String("nats-creds"),
			TLS:                       cmd.Bool("nats-tls"),
			PublishSubjectPattern:     cmd.String("nats-publish-subject-pattern"),
			SubscribeSubjectPattern:   cmd.String("nats-subscribe-subject-pattern"),
			DiagnosticsSubjectPattern: cmd.String("nats-diagnostics-subject-pattern"),
		}, log)
		if err != nil {
			return fmt.Errorf("nats: %w", err)
		}
	} else {
		log.Info("nats-url not set; running without messaging")
	}

	opts := poller.DefaultOptions()
	opts.Interval = time.Duration(cmd.Float("interval") * float64(time.Second))
	opts.WithTimestamp = cmd.Bool("timestamp")
	opts.SinglePublish = cmd.Bool("nats-single")
	opts.Daemon = cmd.Bool("daemon")
	opts.AutoRemove = cmd.Bool("autoremove")

	svc := poller.New(mb, devices, pubAdapter(pub), opts, log)

	rate := time.Duration(cmd.Float("rate") * float64(time.Second))
	diagnosticsRate := time.Duration(cmd.Float("diagnostics-rate") * float64(time.Second))

	runCtx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if delay := cmd.Int("delay"); delay > 0 {
		log.Info("delaying first poll", "seconds", delay)
		select {
		case <-time.After(time.Duration(delay) * time.Second):
		case <-runCtx.Done():
			return nil
		}
	}

	once := cmd.Bool("once")

	var httpPoster *exporter.HTTPPoster
	if url := cmd.String("export-http"); url != "" {
		timeout := time.Duration(cmd.Float("export-http-timeout") * float64(time.Second))
		httpPoster = exporter.NewHTTPPoster(url, timeout)
		log.Info("http export enabled", "url", url, "timeout", timeout)
	}

	err = mainLoop(runCtx, log, svc, pub, opts, rate, diagnosticsRate, once,
		cmd.String("export"), httpPoster)

	if pub != nil {
		drainCtx, drainCancel := context.WithTimeout(context.Background(), 5*time.Second)
		pub.Drain(drainCtx)
		drainCancel()
	}
	return err
}

func mainLoop(
	ctx context.Context,
	log *slog.Logger,
	svc *poller.Service,
	pub *messaging.Handler,
	opts poller.Options,
	rate, diagRate time.Duration,
	once bool,
	exportPath string,
	httpPoster *exporter.HTTPPoster,
) error {
	ticker := time.NewTicker(rate)
	defer ticker.Stop()

	var diagC <-chan time.Time
	if diagRate > 0 {
		t := time.NewTicker(diagRate)
		defer t.Stop()
		diagC = t.C
	}

	var cmdC <-chan messaging.IncomingCommand
	if pub != nil {
		cmdC = pub.Commands()
	}

	doPoll := func() {
		now := time.Now()
		log.Info("polling", "rate_s", rate.Seconds())
		if err := svc.PollAll(ctx); err != nil && !isCanceled(err) {
			log.Error("poll cycle returned error", "err", err)
		}
		if !opts.Daemon {
			svc.PrintResults()
		}
		if pub != nil {
			svc.PublishData(now, false)
		}
		ts := time.Time{}
		if opts.WithTimestamp {
			ts = now
		}
		if exportPath != "" {
			if err := exporter.Export(exportPath, svc.Devices(), ts); err != nil {
				log.Warn("export failed", "err", err)
			}
		}
		if httpPoster != nil {
			if err := httpPoster.Post(ctx, svc.Devices(), ts); err != nil {
				log.Warn("http export failed", "err", err)
			}
		}
	}

	// Initial poll so users see data immediately, then enter the loop.
	doPoll()
	if once {
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			doPoll()
		case <-diagC:
			if pub != nil {
				svc.PublishDiagnostics()
			}
		case cmd := <-cmdC:
			if err := svc.ApplyCommand(cmd); err != nil {
				log.Warn("write command failed", "device", cmd.DeviceName, "err", err)
			} else {
				log.Info("write command applied",
					"device", cmd.DeviceName,
					"object", cmd.Command.ObjectType,
					"address", cmd.Command.Address)
			}
		}
	}
}

func loadDevices(cmd *cli.Command, log *slog.Logger) ([]*domain.Device, error) {
	timeout := time.Duration(cmd.Float("timeout") * float64(time.Second))
	var devices []*domain.Device
	for _, src := range cmd.StringSlice("config") {
		devs, err := config.Load(src, timeout, log)
		if err != nil {
			return nil, fmt.Errorf("config %s: %w", src, err)
		}
		devices = append(devices, devs...)
	}
	if len(devices) == 0 {
		return nil, fmt.Errorf("no devices loaded from any config")
	}
	return devices, nil
}

func buildModbusClient(cmd *cli.Command) (modbus.Client, error) {
	cfg := modbus.Config{
		Framer:       modbus.Framer(strings.ToLower(cmd.String("framer"))),
		Timeout:      time.Duration(cmd.Float("timeout") * float64(time.Second)),
		SerialBaud:   int(cmd.Int("serial-baud")),
		SerialParity: cmd.String("serial-parity"),
	}
	transports := 0
	if h := cmd.String("tcp"); h != "" {
		cfg.Transport = modbus.TransportTCP
		cfg.Host = h
		cfg.Port = int(cmd.Int("tcp-port"))
		transports++
	}
	if h := cmd.String("udp"); h != "" {
		cfg.Transport = modbus.TransportUDP
		cfg.Host = h
		cfg.Port = int(cmd.Int("udp-port"))
		transports++
	}
	if p := cmd.String("serial"); p != "" {
		cfg.Transport = modbus.TransportSerial
		cfg.SerialPort = p
		transports++
	}
	switch transports {
	case 0:
		return nil, fmt.Errorf("no transport specified (use --tcp, --udp or --serial)")
	case 1:
		return modbus.NewClient(cfg)
	}
	return nil, fmt.Errorf("multiple transports specified; pick one of --tcp/--udp/--serial")
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToUpper(level) {
	case "DEBUG":
		lvl = slog.LevelDebug
	case "WARN", "WARNING":
		lvl = slog.LevelWarn
	case "ERROR", "CRITICAL":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})
	return slog.New(h)
}

// pubAdapter returns a poller.Publisher backed by the optional NATS handler.
// When the handler is nil, all publish methods are no-ops so the polling
// loop runs in local-only mode.
func pubAdapter(h *messaging.Handler) poller.Publisher {
	if h == nil {
		return nil
	}
	return h
}

func isCanceled(err error) bool {
	return err == context.Canceled || err == context.DeadlineExceeded
}
