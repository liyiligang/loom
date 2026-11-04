package discovery

import (
	"context"
	"encoding/binary"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/liyiligang/loom/internal/model"
	"github.com/liyiligang/loom/internal/provider/codex"
)

type Candidate struct {
	Address  string `json:"address"`
	Verified bool   `json:"verified"`
	Note     string `json:"note"`
}

type Run struct {
	ID         string      `json:"id"`
	State      string      `json:"state"`
	Checked    int         `json:"checked"`
	Total      int         `json:"total"`
	Limited    bool        `json:"limited"`
	Candidates []Candidate `json:"candidates"`
}

type Scanner struct {
	mu      sync.Mutex
	current Run
	started time.Time
	ports   []int
	ctx     context.Context
	cancel  context.CancelFunc
	workers sync.WaitGroup
}

func New(ctx context.Context, ports []int) *Scanner {
	ctx, cancel := context.WithCancel(ctx)
	return &Scanner{ctx: ctx, cancel: cancel, ports: ports}
}

func (s *Scanner) Close() { s.cancel(); s.workers.Wait() }

func (s *Scanner) Start() Run {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current.State == "scanning" || time.Since(s.started) < 20*time.Second {
		return s.copyLocked()
	}
	s.started = time.Now()
	s.current = Run{ID: model.ID(), State: "scanning", Candidates: []Candidate{}}
	s.workers.Add(1)
	go s.scan(s.current.ID)
	return s.copyLocked()
}

func (s *Scanner) Get() Run { s.mu.Lock(); defer s.mu.Unlock(); return s.copyLocked() }
func (s *Scanner) copyLocked() Run {
	run := s.current
	run.Candidates = append([]Candidate{}, run.Candidates...)
	return run
}

func hosts() ([]string, bool) {
	result := []string{"127.0.0.1"}
	seen := map[string]bool{"127.0.0.1": true}
	limited := false
	interfaces, _ := net.Interfaces()
	for _, iface := range interfaces {
		name := strings.ToLower(iface.Name)
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || strings.Contains(name, "vethernet") || strings.Contains(name, "vpn") || strings.Contains(name, "docker") || strings.Contains(name, "wsl") {
			continue
		}
		addresses, _ := iface.Addrs()
		for _, address := range addresses {
			ip, network, err := net.ParseCIDR(address.String())
			if err != nil || ip.To4() == nil || !ip.IsPrivate() {
				continue
			}
			ones, bits := network.Mask.Size()
			if bits != 32 || ones >= 31 {
				continue
			}
			start := binary.BigEndian.Uint32(network.IP.To4())
			count := uint64(1) << (32 - ones)
			if count > 512 {
				limited = true
			}
			// Check the local address before enumerating the actual subnet.
			if !seen[ip.String()] && len(result) < 512 {
				result = append(result, ip.String())
				seen[ip.String()] = true
			}
			for offset := uint64(1); offset < count-1; offset++ {
				if len(result) >= 512 {
					limited = true
					break
				}
				var candidate [4]byte
				binary.BigEndian.PutUint32(candidate[:], start+uint32(offset))
				text := net.IP(candidate[:]).String()
				if !seen[text] {
					result = append(result, text)
					seen[text] = true
				}
			}
		}
	}
	return result, limited
}

func (s *Scanner) scan(id string) {
	defer s.workers.Done()
	ctx, cancel := context.WithTimeout(s.ctx, 20*time.Second)
	defer cancel()
	addresses, limited := hosts()
	s.mu.Lock()
	s.current.Total, s.current.Limited = len(addresses)*len(s.ports), limited
	s.mu.Unlock()
	jobs := make(chan string)
	var workers sync.WaitGroup
	for range 32 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for address := range jobs {
				candidate := probe(ctx, address)
				s.mu.Lock()
				s.current.Checked++
				if candidate != nil {
					s.current.Candidates = append(s.current.Candidates, *candidate)
				}
				s.mu.Unlock()
			}
		}()
	}
loop:
	for _, host := range addresses {
		for _, port := range s.ports {
			select {
			case jobs <- net.JoinHostPort(host, strconv.Itoa(port)):
			case <-ctx.Done():
				break loop
			}
		}
	}
	close(jobs)
	workers.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current.ID == id {
		s.current.State = "completed"
		if ctx.Err() != nil {
			s.current.Limited = true
		}
	}
}

func probe(ctx context.Context, address string) *Candidate {
	dialer := net.Dialer{Timeout: 500 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil
	}
	conn.Close()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	target := "ws://" + address + "/"
	client, err := codex.Dial(ctx, target, "")
	if err == nil {
		client.Close()
		return &Candidate{Address: target, Verified: true, Note: "Codex app-server verified"}
	}
	if strings.Contains(err.Error(), "authentication required") {
		return &Candidate{Address: target, Verified: false, Note: "Authentication required; provider identity is unverified"}
	}
	return nil
}
