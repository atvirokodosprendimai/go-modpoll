// Package config loads modpoll's CSV register/device description from a local
// path or an HTTP(S) URL into the domain model.
package config

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/atvirokodosprendimai/go-modpoll/internal/domain"
)

const (
	minDeviceCols = 3
	minPollCols   = 5
	minRefCols    = 5
)

// Load reads and parses the configuration referenced by source. The source
// may be an HTTP(S) URL or a local filesystem path.
func Load(source string, timeout time.Duration, log *slog.Logger) ([]*domain.Device, error) {
	r, closer, err := open(source, timeout)
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.Comment = '#'
	cr.LazyQuotes = true

	devices, err := parseRows(cr, log)
	if err != nil {
		return nil, err
	}
	if len(devices) == 0 {
		return nil, errors.New("no devices defined in config")
	}
	return devices, nil
}

func open(source string, timeout time.Duration) (io.Reader, io.Closer, error) {
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		client := &http.Client{Timeout: timeout}
		resp, err := client.Get(source)
		if err != nil {
			return nil, nil, fmt.Errorf("fetch %s: %w", source, err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, nil, fmt.Errorf("fetch %s: HTTP %d", source, resp.StatusCode)
		}
		return resp.Body, resp.Body, nil
	}
	f, err := os.Open(source)
	if err != nil {
		return nil, nil, fmt.Errorf("open %s: %w", source, err)
	}
	return f, f, nil
}

func parseRows(cr *csv.Reader, log *slog.Logger) ([]*domain.Device, error) {
	var (
		devices []*domain.Device
		current *domain.Device
		poller  *domain.Poller
	)
	for {
		row, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			// Allow per-row recovery on length mismatches by reading next.
			var parseErr *csv.ParseError
			if errors.As(err, &parseErr) {
				log.Warn("skipping malformed row", "err", err)
				continue
			}
			return nil, err
		}
		if isBlank(row) {
			continue
		}
		kind := strings.ToLower(strings.TrimSpace(row[0]))
		switch {
		case strings.Contains(kind, "device"):
			dev, err := parseDevice(row)
			if err != nil {
				log.Warn("skipping device row", "err", err)
				continue
			}
			devices = append(devices, dev)
			current = dev
			poller = nil
		case strings.Contains(kind, "poll"):
			if current == nil {
				log.Warn("poll row before device row, skipping")
				continue
			}
			p, err := parsePoller(row)
			if err != nil {
				log.Warn("skipping poll row", "err", err)
				continue
			}
			current.Pollers = append(current.Pollers, p)
			poller = p
		case strings.Contains(kind, "ref"):
			if current == nil || poller == nil {
				log.Debug("ref row without device/poller, skipping")
				continue
			}
			ref, err := parseReference(row)
			if err != nil {
				log.Warn("skipping ref row", "err", err)
				continue
			}
			if !ref.InRange(poller.StartAddress, poller.Size) {
				log.Warn("reference outside poller range, skipping", "name", ref.Name)
				continue
			}
			if ref.Readable() {
				poller.AddReadable(ref)
			}
			current.AddReference(ref)
		default:
			log.Debug("unknown row kind, skipping", "kind", kind)
		}
	}
	return devices, nil
}

func parseDevice(row []string) (*domain.Device, error) {
	if len(row) < minDeviceCols {
		return nil, fmt.Errorf("device row needs %d columns, got %d", minDeviceCols, len(row))
	}
	name := strings.TrimSpace(row[1])
	if name == "" {
		return nil, errors.New("device name is empty")
	}
	id, err := parseInt(row[2])
	if err != nil {
		return nil, fmt.Errorf("invalid device id %q: %w", row[2], err)
	}
	if id < 0 || id > 255 {
		return nil, fmt.Errorf("device id %d out of range 0..255", id)
	}
	return domain.NewDevice(name, uint8(id)), nil
}

func parsePoller(row []string) (*domain.Poller, error) {
	if len(row) < minPollCols {
		return nil, fmt.Errorf("poll row needs %d columns, got %d", minPollCols, len(row))
	}
	fc, err := domain.ParseObjectType(strings.ToLower(strings.TrimSpace(row[1])))
	if err != nil {
		return nil, err
	}
	start, err := parseInt(row[2])
	if err != nil {
		return nil, fmt.Errorf("invalid start address: %w", err)
	}
	size, err := parseInt(row[3])
	if err != nil {
		return nil, fmt.Errorf("invalid size: %w", err)
	}
	endian, err := domain.ParseEndian(row[4])
	if err != nil {
		return nil, err
	}
	return domain.NewPoller(fc, start, size, endian)
}

func parseReference(row []string) (*domain.Reference, error) {
	if len(row) < minRefCols {
		return nil, fmt.Errorf("ref row needs %d columns, got %d", minRefCols, len(row))
	}
	name := strings.TrimSpace(row[1])
	address := strings.TrimSpace(row[2])
	dtype := domain.Normalize(row[3])
	rw := strings.TrimSpace(row[4])
	unit := ""
	if len(row) > 5 {
		unit = strings.TrimSpace(row[5])
	}
	scale := 0.0
	if len(row) > 6 && strings.TrimSpace(row[6]) != "" {
		v, err := strconv.ParseFloat(strings.TrimSpace(row[6]), 64)
		if err == nil {
			scale = v
		}
	}
	return domain.NewReference(name, address, dtype, rw, unit, scale)
}

func isBlank(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

func parseInt(s string) (int, error) {
	s = strings.TrimSpace(s)
	v, err := strconv.ParseInt(s, 0, 64)
	if err != nil {
		return 0, err
	}
	return int(v), nil
}
