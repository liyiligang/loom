package local

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/liyiligang/loom/internal/provider"
	"github.com/liyiligang/loom/internal/provider/claude"
	"github.com/liyiligang/loom/internal/provider/codex"
	"github.com/liyiligang/loom/internal/provider/stdio"
)

type Agent struct {
	Provider string `json:"provider"`
	Name     string `json:"name"`
	Path     string `json:"path,omitempty"`
	Version  string `json:"version,omitempty"`
	State    string `json:"state"`
	Error    string `json:"error,omitempty"`
	Node     string `json:"-"`
}

type Manager interface {
	Scan(context.Context) []Agent
	Connect(context.Context, Agent) (provider.Connection, error)
}
type Runtime struct {
	DataDirectory string
	Home          string
	SearchPath    string
}

func New(directory string) *Runtime {
	home, _ := os.UserHomeDir()
	return &Runtime{DataDirectory: directory, Home: home, SearchPath: os.Getenv("PATH")}
}
func (r *Runtime) candidates(name string) []string {
	dirs := filepath.SplitList(r.SearchPath)
	dirs = append(dirs, filepath.Join(r.Home, ".local", "bin"), filepath.Join(r.Home, ".npm-global", "bin"), "/opt/homebrew/bin", "/usr/local/bin", filepath.Join(r.Home, ".volta", "bin"), filepath.Join(r.Home, ".asdf", "shims"), filepath.Join(r.Home, "Library", "pnpm"), filepath.Join(r.Home, ".local", "share", "pnpm"))
	if runtime.GOOS == "windows" {
		dirs = append(dirs, filepath.Join(os.Getenv("APPDATA"), "npm"), filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "WinGet", "Links"), filepath.Join(os.Getenv("ProgramFiles"), "nodejs"))
	}
	for _, pattern := range []string{filepath.Join(r.Home, ".nvm", "versions", "node", "*", "bin"), filepath.Join(r.Home, ".fnm", "node-versions", "*", "installation", "bin"), filepath.Join(r.Home, "Library", "Application Support", "fnm", "node-versions", "*", "installation", "bin"), filepath.Join(r.Home, ".asdf", "installs", "nodejs", "*", "bin")} {
		matches, _ := filepath.Glob(pattern)
		sort.Sort(sort.Reverse(sort.StringSlice(matches)))
		dirs = append(dirs, matches...)
	}
	names := []string{name}
	if runtime.GOOS == "windows" {
		names = []string{name + ".exe", name + ".cmd", name + ".bat"}
	}
	if runtime.GOOS == "darwin" && name == "codex" {
		for _, root := range []string{"/Applications", filepath.Join(r.Home, "Applications")} {
			dirs = append(dirs, filepath.Join(root, "Codex.app", "Contents", "Resources"), filepath.Join(root, "Codex.app", "Contents", "Resources", "codex-cli"), filepath.Join(root, "ChatGPT.app", "Contents", "Resources", "codex-cli", "CodexCLI.app", "Contents", "MacOS"))
		}
	}
	var paths []string
	seen := map[string]bool{}
	for _, directory := range dirs {
		if directory == "" {
			continue
		}
		for _, name := range names {
			path := filepath.Join(directory, name)
			if !filepath.IsAbs(path) {
				continue
			}
			if !seen[path] {
				paths = append(paths, path)
				seen[path] = true
			}
		}
	}
	return paths
}
func (r *Runtime) find(ctx context.Context, name string, timeout time.Duration) (string, string) {
	probes := 0
	for _, candidate := range r.candidates(name) {
		if ctx.Err() != nil || probes >= 12 {
			break
		}
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		if runtime.GOOS != "windows" && info.Mode()&0111 == 0 {
			continue
		}
		probes++
		check, cancel := context.WithTimeout(ctx, timeout)
		command := stdio.Command(check, candidate, "--version")
		output := &versionOutput{}
		command.Stdout = output
		command.Stderr = io.Discard
		err = command.Run()
		cancel()
		version := strings.TrimSpace(string(output.data))
		if err == nil && !output.excess && len(version) > 0 && len(version) < 256 {
			lower := strings.ToLower(version)
			if (name == "codex" && strings.Contains(lower, "codex")) || (name == "claude" && strings.Contains(lower, "claude")) || (name == "node" && supportedNode(version)) {
				return candidate, version
			}
		}
	}
	return "", ""
}
func (r *Runtime) Scan(ctx context.Context) []Agent {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	agents := []Agent{{Provider: "codex", Name: "Codex", State: "missing"}, {Provider: "claude", Name: "Claude Code", State: "missing"}}
	var scans sync.WaitGroup
	for i := range agents {
		scans.Add(1)
		go func() {
			defer scans.Done()
			agents[i].Path, agents[i].Version = r.find(ctx, agents[i].Provider, 2*time.Second)
			if agents[i].Path != "" {
				agents[i].State = "available"
			}
		}()
	}
	scans.Wait()
	if agents[1].State == "available" {
		agents[1].Node, _ = r.find(ctx, "node", 2*time.Second)
		if agents[1].Node == "" {
			agents[1].State = "unavailable"
			agents[1].Error = "Node.js 22 or later is required to run Claude Code from Loom."
		}
		if extension := strings.ToLower(filepath.Ext(agents[1].Path)); extension == ".cmd" || extension == ".bat" {
			if _, err := os.Stat(filepath.Join(filepath.Dir(agents[1].Path), "node_modules", "@anthropic-ai", "claude-code", "cli.js")); err != nil {
				agents[1].State, agents[1].Error = "unavailable", "The installed Claude Code command shim could not be resolved. Install the native Claude Code CLI and detect again."
			}
		}
	}
	return agents
}
func (r *Runtime) Connect(ctx context.Context, agent Agent) (provider.Connection, error) {
	if agent.Provider == "codex" {
		return codex.Start(ctx, agent.Path)
	}
	if agent.Provider != "claude" {
		return nil, errors.New("The local agent provider is unsupported.")
	}
	executable := agent.Path
	if extension := strings.ToLower(filepath.Ext(executable)); extension == ".cmd" || extension == ".bat" {
		executable = filepath.Join(filepath.Dir(executable), "node_modules", "@anthropic-ai", "claude-code", "cli.js")
	}
	return claude.Start(ctx, agent.Node, executable, r.DataDirectory)
}

// Version probes never retain unbounded output from a broken executable.
type versionOutput struct {
	data   []byte
	excess bool
}

func (v *versionOutput) Write(data []byte) (int, error) {
	n := len(data)
	remaining := 1024 - len(v.data)
	if n > remaining {
		v.excess = true
		data = data[:remaining]
	}
	v.data = append(v.data, data...)
	return n, nil
}
func supportedNode(version string) bool {
	if !strings.HasPrefix(version, "v") {
		return false
	}
	major, err := strconv.Atoi(strings.Split(strings.TrimPrefix(version, "v"), ".")[0])
	return err == nil && major >= 22
}
