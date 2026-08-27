// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package observe

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExporterFromEnv_EmptyIsNoop(t *testing.T) {
	t.Setenv("GOSO_OTEL_ENDPOINT", "")
	t.Setenv("GRAFANA_CLOUD_API_KEY", "should-not-be-used")
	e := ExporterFromEnv()
	if _, ok := e.(NoopExporter); !ok {
		t.Fatalf("want NoopExporter, got %T", e)
	}
	if err := e.Export(context.Background(), []Span{{Kind: KindAgent, Name: "agent"}}); err != nil {
		t.Fatal(err)
	}
}

func TestFakeExporter_Records(t *testing.T) {
	fake := &FakeExporter{}
	obs := NewWithWriter(io.Discard)
	obs.SetExporter(fake)
	obs.RecordSpans([]Span{{TraceID: "t1", SpanID: "s1", Kind: KindAgent, Name: "agent"}})
	all := fake.All()
	if len(all) != 1 || all[0].Kind != KindAgent {
		t.Fatalf("fake %+v", all)
	}
	if obs.SpanTrees().Len() != 1 {
		t.Fatalf("ring %d", obs.SpanTrees().Len())
	}
}

func TestHTTPExporter_PostsJSONNoGrafanaHeader(t *testing.T) {
	t.Setenv("GRAFANA_CLOUD_API_KEY", "graf-secret")
	var gotAuth []string
	var gotCT string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = append([]string{}, r.Header.Values("Authorization")...)
		gotCT = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		for k := range r.Header {
			if strings.Contains(strings.ToLower(k), "grafana") {
				t.Errorf("grafana header %s", k)
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv("GOSO_OTEL_ENDPOINT", srv.URL)
	e := ExporterFromEnv()
	httpE, ok := e.(*HTTPExporter)
	if !ok {
		t.Fatalf("want *HTTPExporter, got %T", e)
	}
	httpE.Client = srv.Client()
	err := e.Export(context.Background(), []Span{{
		TraceID: "aa", SpanID: "bb", Kind: KindAgent, Name: "agent", CacheReadTokens: 0,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if gotCT != "application/json" {
		t.Fatalf("content-type %s", gotCT)
	}
	if len(gotAuth) != 0 {
		t.Fatalf("authorization %v", gotAuth)
	}
	if bytes.Contains(gotBody, []byte("graf-secret")) {
		t.Fatal("grafana key leaked into OTLP body")
	}
	var payload map[string]any
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload["resourceSpans"]; !ok {
		t.Fatalf("body %s", gotBody)
	}
}

func TestNew_UsesNoopWhenEndpointEmpty(t *testing.T) {
	t.Setenv("GOSO_OTEL_ENDPOINT", "")
	obs := New()
	if _, ok := obs.exporter.(NoopExporter); !ok {
		t.Fatalf("prod New with empty endpoint: %T", obs.exporter)
	}
}
