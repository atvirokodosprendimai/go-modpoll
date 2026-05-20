// Package exporter writes the latest decoded device data to a JSON file.
package exporter

import (
	"encoding/json"
	"os"
	"time"

	"github.com/atvirokodosprendimai/go-modpoll/internal/domain"
)

// Export writes all current reference values to path as JSON. When timestamp
// is non-zero it is added to each device's payload.
func Export(path string, devices []*domain.Device, timestamp time.Time) error {
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
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
