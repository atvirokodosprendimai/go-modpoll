// Package exporter writes the latest decoded device data to a JSON file or
// POSTs it to an HTTP endpoint.
package exporter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/atvirokodosprendimai/go-modpoll/internal/domain"
)

// snapshot builds the JSON shape shared by Export and PostHTTP.
func snapshot(devices []*domain.Device, timestamp time.Time) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, dev := range devices {
		row := map[string]any{}
		for _, ref := range dev.References {
			row[ref.Name] = ref.Val
		}
		if !timestamp.IsZero() {
			row["timestamp"] = timestamp.UTC().Format(time.RFC3339Nano)
		}
		out[dev.Name] = row
	}
	return out
}

// Export writes all current reference values to path as JSON.
func Export(path string, devices []*domain.Device, timestamp time.Time) error {
	data, err := json.MarshalIndent(snapshot(devices, timestamp), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// HTTPPoster posts the data snapshot as JSON to a configured URL on every
// successful poll cycle. The zero value is unusable — construct with
// NewHTTPPoster.
type HTTPPoster struct {
	url    string
	client *http.Client
}

// NewHTTPPoster builds a poster pinned to url. A timeout of 0 falls back to
// 10 seconds.
func NewHTTPPoster(url string, timeout time.Duration) *HTTPPoster {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &HTTPPoster{url: url, client: &http.Client{Timeout: timeout}}
}

// Post sends a compact JSON payload via HTTP POST. The HTTP status must be
// 2xx for the call to succeed; non-2xx responses are reported as errors and
// the body (truncated to 256 bytes) is included in the message so server-side
// rejections show up in modpoll's logs.
func (h *HTTPPoster) Post(ctx context.Context, devices []*domain.Device, timestamp time.Time) error {
	body, err := json.Marshal(snapshot(devices, timestamp))
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "modpoll/1")
	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		preview, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("http %s %s: %s", resp.Status, h.url, bytes.TrimSpace(preview))
	}
	io.Copy(io.Discard, resp.Body)
	return nil
}
