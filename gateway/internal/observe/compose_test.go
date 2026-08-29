// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package observe

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestComposeYml_OTelPortNoGrafanaCloud(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	raw, err := os.ReadFile(filepath.Join(root, "compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if !strings.Contains(body, "4318") {
		t.Fatal("compose.yml missing OTLP HTTP 4318")
	}
	if !strings.Contains(body, "- otel") {
		t.Fatal("compose.yml missing profile otel")
	}
	if !strings.Contains(body, "http://jaeger:4318/v1/traces") {
		t.Fatal("compose.yml missing compose-network GOSO_OTEL_ENDPOINT")
	}
	lower := strings.ToLower(body)
	for _, bad := range []string{"grafana-cloud", "grafana_cloud", "api-key", "apikey"} {
		if strings.Contains(lower, bad) {
			t.Fatalf("compose.yml contains vendor name %q", bad)
		}
	}
}
