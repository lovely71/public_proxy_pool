package validator

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPerformCheckIPRequest_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("8.8.8.8\n"))
	}))
	defer srv.Close()

	ok, status, latencyMS, finalURL, exitIP, err := performCheckIPRequest(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatalf("performCheckIPRequest failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if status != http.StatusOK {
		t.Fatalf("unexpected status: %d", status)
	}
	if latencyMS < 0 {
		t.Fatalf("unexpected latency: %d", latencyMS)
	}
	if finalURL != srv.URL {
		t.Fatalf("unexpected final url: %s", finalURL)
	}
	if exitIP != "8.8.8.8" {
		t.Fatalf("unexpected exit ip: %s", exitIP)
	}
}

func TestPerformCheckIPRequest_BadBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-an-ip"))
	}))
	defer srv.Close()

	ok, status, _, _, exitIP, err := performCheckIPRequest(context.Background(), srv.Client(), srv.URL)
	if err == nil {
		t.Fatalf("expected error")
	}
	if ok {
		t.Fatalf("expected ok=false")
	}
	if status != http.StatusOK {
		t.Fatalf("unexpected status: %d", status)
	}
	if exitIP != "" {
		t.Fatalf("unexpected exit ip: %s", exitIP)
	}
}

func TestPerformCheckIPRequest_CloudflareTrace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("fl=123f456\nip=1.1.1.1\nloc=US\n"))
	}))
	defer srv.Close()

	ok, status, _, finalURL, exitIP, err := performCheckIPRequest(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatalf("performCheckIPRequest failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if status != http.StatusOK {
		t.Fatalf("unexpected status: %d", status)
	}
	if finalURL != srv.URL {
		t.Fatalf("unexpected final url: %s", finalURL)
	}
	if exitIP != "1.1.1.1" {
		t.Fatalf("unexpected exit ip: %s", exitIP)
	}
}

func TestPerformOKRequest_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	ok, status, latencyMS, finalURL, err := performOKRequest(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatalf("performOKRequest failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if status != http.StatusOK {
		t.Fatalf("unexpected status: %d", status)
	}
	if latencyMS < 0 {
		t.Fatalf("unexpected latency: %d", latencyMS)
	}
	if finalURL != srv.URL {
		t.Fatalf("unexpected final url: %s", finalURL)
	}
}

func TestPerformOKRequest_BadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("nope"))
	}))
	defer srv.Close()

	ok, status, _, _, err := performOKRequest(context.Background(), srv.Client(), srv.URL)
	if err == nil {
		t.Fatalf("expected error")
	}
	if ok {
		t.Fatalf("expected ok=false")
	}
	if status != http.StatusForbidden {
		t.Fatalf("unexpected status: %d", status)
	}
}

func TestParseIPFromText(t *testing.T) {
	if got := parseIPFromText("Current IP: 2001:4860:4860::8888"); got != "2001:4860:4860::8888" {
		t.Fatalf("unexpected ipv6 parse result: %s", got)
	}
}
