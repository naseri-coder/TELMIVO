package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This is a SOURCE CONTRACT, not a claim that Docker/Go1.27.2 was run.
// It prevents somebody from accidentally removing release guard invocations.
func TestLICA9Phase51JBackendDockerfileKeepsReleaseGuard(t *testing.T) {
	candidates := []string{
		filepath.Join("..", "..", "deployments", "docker", "backend.Dockerfile"),
		filepath.Join("deployments", "docker", "backend.Dockerfile"),
	}
	var source []byte
	var err error
	for _, path := range candidates {
		source, err = os.ReadFile(path)
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("real production Backend Dockerfile unavailable: %v", err)
	}
	text := string(source)
	for _, fragment := range []string{
		"FROM golang:1.27.2-bookworm AS build",
		"COPY scripts/release/phase51j_guard.go",
		"GOFLAGS= GOWORK=off go list -json -deps ./cmd/...",
		"go run ./scripts/release/phase51j_guard.go selected",
		"GOFLAGS= GOWORK=off CGO_ENABLED=0 go build -trimpath",
		"go run ./scripts/release/phase51j_guard.go binary /out/$app go1.27.2",
		"USER 10001:10001",
	} {
		if !strings.Contains(text, fragment) {
			t.Errorf("production build lost security requirement: %q", fragment)
		}
	}
	if strings.Contains(text, "go build -tags=lic_a9_browser_test") ||
		strings.Contains(text, "GOFLAGS=-tags") {
		t.Fatal("production Dockerfile enables test-only V2 fixture code")
	}
}
