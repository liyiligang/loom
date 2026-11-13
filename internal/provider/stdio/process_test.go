package stdio

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"
	"time"
)

func TestStdioChild(t *testing.T) {
	mode := os.Args[len(os.Args)-1]
	if mode == "loom-lines" {
		for i := range 20 {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]int{"sequence": i})
		}
		os.Exit(0)
	}
	if mode == "loom-wait" {
		for {
			time.Sleep(time.Hour)
		}
	}
}
func TestFinalOutputSurvivesProcessExit(t *testing.T) {
	process, err := Start(context.Background(), os.Args[0], []string{"-test.run=TestStdioChild", "--", "loom-lines"}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer process.Close()
	<-process.done
	for i := range 20 {
		var value struct{ Sequence int }
		if err := process.ReadJSON(&value); err != nil {
			t.Fatal(err)
		}
		if value.Sequence != i {
			t.Fatal("output order changed")
		}
	}
	if err := process.ReadJSON(&struct{}{}); err != io.EOF {
		t.Fatalf("expected EOF, got %v", err)
	}
}
func TestCloseStopsOwnedChildAndUnblocksReads(t *testing.T) {
	process, err := Start(context.Background(), os.Args[0], []string{"-test.run=TestStdioChild", "--", "loom-wait"}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	read := make(chan error, 1)
	go func() { read <- process.ReadJSON(&struct{}{}) }()
	process.Close()
	process.Close()
	select {
	case err := <-read:
		if err == nil {
			t.Fatal("read unexpectedly succeeded")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("child read did not stop")
	}
	if process.cmd.ProcessState == nil {
		t.Fatal("child still running")
	}
}
