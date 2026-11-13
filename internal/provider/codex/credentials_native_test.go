package codex

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/liyiligang/loom/internal/credentialtool"
	"github.com/liyiligang/loom/internal/model"
	"github.com/liyiligang/loom/internal/provider"
)

type nativeCredentialAccess struct{}

func (nativeCredentialAccess) ListSessionCredentials(string) ([]model.Secret, error) {
	return []model.Secret{{ID: "fixture", Name: "Fixture credential", Kind: "token"}}, nil
}
func (nativeCredentialAccess) LeaseSessionCredential(context.Context, string, string) (credentialtool.Lease, error) {
	panic("native probe must not execute HTTP requests")
}

// This opt-in probe initializes installed Codex and discovers tools without a model turn.
func TestInstalledCodexDiscoversScopedCredentialTools(t *testing.T) {
	executable := os.Getenv("LOOM_NATIVE_CODEX")
	if executable == "" {
		t.Skip("requires an explicitly selected local Codex CLI")
	}
	t.Setenv("CODEX_HOME", t.TempDir())
	broker, err := credentialtool.New(nativeCredentialAccess{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Close()
	tools := broker.Session("probe-session")
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	client, err := Start(ctx, executable)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "AGENTS.md"), []byte("Use English."), 0600); err != nil {
		t.Fatal(err)
	}
	session, err := client.CreateSession(ctx, provider.SessionOptions{Cwd: directory, Tools: &tools})
	if err != nil {
		t.Fatal(err)
	}
	var status struct {
		Data []struct {
			Name  string         `json:"name"`
			Tools map[string]any `json:"tools"`
		}
	}
	if err := client.Call(ctx, "mcpServerStatus/list", map[string]any{"threadId": session.ID, "serverName": "loom_credentials"}, &status); err != nil {
		t.Fatal(err)
	}
	if len(status.Data) != 1 || len(status.Data[0].Tools) != 2 {
		t.Fatalf("expected two credential tools, got %+v", status)
	}
	var result struct {
		IsError bool `json:"isError"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := client.Call(ctx, "mcpServer/tool/call", map[string]any{"threadId": session.ID, "server": "loom_credentials", "tool": "list_credentials", "arguments": map[string]any{}}, &result); err != nil {
		t.Fatal(err)
	}
	if result.IsError || len(result.Content) != 1 {
		t.Fatal("native MCP metadata call failed")
	}
	t.Log("Installed Codex connected to scoped MCP tools and listed fixture metadata; no model turn or platform request was started.")
}
