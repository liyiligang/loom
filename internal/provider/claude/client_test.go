package claude

import (
	"context"
	"os/exec"
	"testing"

	"github.com/liyiligang/loom/internal/provider"
)

func TestEmbeddedBridgeInitializesWithoutLaunchingClaude(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node unavailable")
	}
	client, err := Start(context.Background(), node, "/nonexistent/claude", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	session, err := client.CreateSession(context.Background(), provider.SessionOptions{Cwd: t.TempDir()})
	if err != nil || session.ID == "" {
		t.Fatalf("empty session: %+v %v", session, err)
	}
	synchronized, err := client.SynchronizeSession(context.Background(), provider.SessionReference{ID: session.ID, Subscribed: true, Empty: true})
	if err != nil || synchronized.ID != session.ID || len(synchronized.Turns) != 0 {
		t.Fatalf("synchronization: %+v %v", synchronized, err)
	}
}
