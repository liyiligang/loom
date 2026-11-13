package stdio

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// Process owns one child and its pipes. Only children started here are stopped.
type Process struct {
	cmd    *exec.Cmd
	input  io.WriteCloser
	lines  *bufio.Scanner
	cancel context.CancelFunc
	done   chan struct{}
	output io.ReadCloser
	once   sync.Once
}

func Start(ctx context.Context, executable string, args []string, directory string) (*Process, error) {
	lifetime, cancel := context.WithCancel(ctx)
	cmd := Command(lifetime, executable, args...)
	cmd.Dir = directory
	input, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	output, writer, err := os.Pipe()
	if err != nil {
		input.Close()
		cancel()
		return nil, err
	}
	// Keep protocol stdout separate from CLI diagnostics, which can contain account data.
	cmd.Stderr = io.Discard
	cmd.Stdout = writer
	if err := cmd.Start(); err != nil {
		input.Close()
		output.Close()
		writer.Close()
		cancel()
		return nil, fmt.Errorf("Could not start the local agent: %w", err)
	}
	_ = writer.Close()
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	process := &Process{cmd: cmd, input: input, lines: scanner, cancel: cancel, done: make(chan struct{}), output: output}
	go func() { _ = cmd.Wait(); close(process.done) }()
	return process, nil
}

func (p *Process) ReadJSON(value any) error {
	for p.lines.Scan() {
		raw := p.lines.Bytes()
		if len(raw) == 0 {
			continue
		}
		return json.Unmarshal(raw, value)
	}
	if err := p.lines.Err(); err != nil {
		return err
	}
	return io.EOF
}
func (p *Process) WriteJSON(value any) error { return json.NewEncoder(p.input).Encode(value) }
func (p *Process) Close() error {
	p.once.Do(func() {
		// Stop descendants even if their parent already exited and left a pipe open.
		_ = p.cmd.Cancel()
		p.cancel()
		_ = p.input.Close()
		_ = p.output.Close()
		<-p.done
	})
	return nil
}

// Command supports native executables and npm command shims on Windows.
func Command(ctx context.Context, executable string, args ...string) *exec.Cmd {
	cmd := command(ctx, executable, args)
	cmd.WaitDelay = 2 * time.Second
	configure(cmd)
	return cmd
}
