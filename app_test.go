package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFailEmbeddedServerStartupReadiesFallbackHandler(t *testing.T) {
	app := NewApp()
	app.failEmbeddedServerStartup("test startup failure")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces", nil)
	rec := httptest.NewRecorder()

	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status: got %d want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if rec.Body.String() == "" {
		t.Fatal("expected fallback error body")
	}
}

func TestSignalReadyIsIdempotent(t *testing.T) {
	app := NewApp()

	app.signalReady()
	app.signalReady()
}
