package exporter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/atvirokodosprendimai/go-modpoll/internal/domain"
)

func sampleDevices() []*domain.Device {
	d := domain.NewDevice("dev1", 1)
	ref, _ := domain.NewReference("temp", "40000", domain.TypeFloat32, "r", "C", 0)
	ref.UpdateValue(float32(21.5))
	d.AddReference(ref)
	return []*domain.Device{d}
}

func TestHTTPPoster_Post(t *testing.T) {
	var (
		gotBody        []byte
		gotContentType string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method: got %s want POST", r.Method)
		}
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := NewHTTPPoster(srv.URL, 2*time.Second)
	if err := p.Post(context.Background(), sampleDevices(), time.Time{}); err != nil {
		t.Fatalf("post: %v", err)
	}

	if gotContentType != "application/json" {
		t.Errorf("content-type: got %q", gotContentType)
	}
	var decoded map[string]map[string]any
	if err := json.Unmarshal(gotBody, &decoded); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, gotBody)
	}
	if _, ok := decoded["dev1"]; !ok {
		t.Errorf("missing device in payload: %s", gotBody)
	}
	if _, ok := decoded["dev1"]["temp"]; !ok {
		t.Errorf("missing reference value: %s", gotBody)
	}
}

func TestHTTPPoster_PostNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		io.WriteString(w, "upstream down")
	}))
	defer srv.Close()

	p := NewHTTPPoster(srv.URL, 2*time.Second)
	err := p.Post(context.Background(), sampleDevices(), time.Time{})
	if err == nil {
		t.Fatal("expected error on non-2xx response")
	}
	if !strings.Contains(err.Error(), "upstream down") {
		t.Errorf("expected body in error, got %v", err)
	}
}
