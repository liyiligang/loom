package local

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestDetectedPathIsVerifiedAndPreferred(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable fixture")
	}
	directory := t.TempDir()
	home := t.TempDir()
	executable := filepath.Join(directory, "codex")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf 'codex-cli fixture\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	detector := &Runtime{Home: home, SearchPath: directory + string(os.PathListSeparator) + directory}
	found, version := detector.find(context.Background(), "codex", 5*time.Second)
	if found != executable || version != "codex-cli fixture" {
		t.Fatalf("wrong detected executable: %s %s", found, version)
	}
	seen := map[string]bool{}
	for _, candidate := range detector.candidates("codex") {
		if seen[candidate] || !filepath.IsAbs(candidate) {
			t.Fatal("duplicate or relative candidate")
		}
		seen[candidate] = true
	}
}
func TestNodeVersionRequirementAndBoundedProbeOutput(t *testing.T) {
	for version, want := range map[string]bool{"v22.0.0": true, "v24.18.1": true, "v20.19.0": false, "v18.1.0": false, "garbage": false, "vnot-node": false} {
		if supportedNode(version) != want {
			t.Fatalf("Node requirement: %s", version)
		}
	}
	output := &versionOutput{}
	if n, err := output.Write([]byte(strings.Repeat("x", 10000))); n != 10000 || err != nil || len(output.data) != 1024 || !output.excess {
		t.Fatal("version probe output was not bounded")
	}
}
func TestCancelledDetectionDoesNotExecuteCandidates(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	detector := New(t.TempDir())
	agents := detector.Scan(ctx)
	if len(agents) != 2 || agents[0].State != "missing" || agents[1].State != "missing" {
		t.Fatal("cancelled detection returned installed agents")
	}
}
