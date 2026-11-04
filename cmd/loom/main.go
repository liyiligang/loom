package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/liyiligang/loom/internal/controller"
	"github.com/liyiligang/loom/internal/credentials"
	"github.com/liyiligang/loom/internal/discovery"
	"github.com/liyiligang/loom/internal/httpapi"
	"github.com/liyiligang/loom/internal/storage"
	"github.com/liyiligang/loom/web"
)

func main() {
	if err := run(); err != nil {
		slog.Error("Loom stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	bind := flag.String("bind", "127.0.0.1", "Loopback address for the browser server")
	port := flag.Int("port", 7331, "Browser server port")
	data := flag.String("data", "data", "Directory for SQLite and service credentials")
	probePorts := flag.String("probe-ports", "3000", "Comma-separated Codex discovery ports")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q; use --help for supported options", flag.Arg(0))
	}
	ip := net.ParseIP(*bind)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("--bind must be a loopback IP address; browser access authentication is not implemented yet")
	}
	if *port < 1 || *port > 65535 {
		return fmt.Errorf("--port must be between 1 and 65535")
	}
	var ports []int
	for _, value := range strings.Split(*probePorts, ",") {
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || parsed < 1 || parsed > 65535 {
			return fmt.Errorf("invalid discovery port %q", value)
		}
		ports = append(ports, parsed)
	}
	if len(ports) > 8 {
		return fmt.Errorf("configure at most eight discovery ports")
	}
	address := net.JoinHostPort(*bind, strconv.Itoa(*port))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("listen on %s: %w; choose another --port if it is in use", address, err)
	}
	defer listener.Close()
	store, err := storage.Open(filepath.Join(*data, "loom.db"))
	if err != nil {
		return err
	}
	defer store.Close()
	secrets, err := credentials.New(filepath.Join(*data, "credentials"))
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	service, err := controller.New(ctx, store, secrets)
	if err != nil {
		return err
	}
	defer service.Close()
	scanner := discovery.New(ctx, ports)
	defer scanner.Close()
	server := &http.Server{Handler: httpapi.New(ctx, service, scanner, web.Assets(), address),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 16 << 10}
	serveError := make(chan error, 1)
	go func() { serveError <- server.Serve(listener) }()
	service.Start()
	fmt.Printf("Loom is ready at http://%s\n", address)
	var result error
	select {
	case <-ctx.Done():
	case result = <-serveError:
	case result = <-service.Fatal:
	}
	stop()
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdown); err != nil && result == nil {
		result = err
	}
	if result == http.ErrServerClosed {
		return nil
	}
	return result
}
