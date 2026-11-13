package controller

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/liyiligang/loom/internal/local"
	"github.com/liyiligang/loom/internal/model"
)

type Option func(*Service)

func WithLocalManager(manager local.Manager) Option {
	return func(service *Service) { service.localManager = manager }
}

type LocalSnapshot struct {
	State            string        `json:"state"`
	Confirmed        bool          `json:"confirmed"`
	Agents           []local.Agent `json:"agents"`
	DefaultDirectory string        `json:"defaultDirectory"`
}

func (s *Service) ScanLocal() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.localState == "scanning" || s.ctx.Err() != nil {
		return
	}
	s.localState = "scanning"
	_ = s.commitLocked("local.updated", "")
	s.workers.Add(1)
	go func() {
		defer s.workers.Done()
		agents := s.localManager.Scan(s.ctx)
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.ctx.Err() != nil {
			return
		}
		s.localAgents = agents
		s.localState = "ready"
		var values []any
		if s.data.LocalSetup.Confirmed {
			values = s.addLocalEndpointsLocked(false)
		}
		_ = s.commitLocked("local.updated", "", values...)
		for _, endpoint := range s.data.Endpoints {
			if endpoint.Local && endpoint.AutoConnect {
				s.connectLocked(endpoint)
			}
		}
	}()
}
func (s *Service) addLocalEndpointsLocked(enable bool) []any {
	var values []any
	for _, agent := range s.localAgents {
		if agent.State != "available" {
			continue
		}
		id := "local-" + agent.Provider
		endpoint := s.endpoint(id)
		if endpoint == nil {
			endpoint = &model.Endpoint{ID: id, Name: agent.Name, Provider: agent.Provider, Local: true, Address: "local://" + agent.Provider, AutoConnect: true, State: "offline"}
			s.data.Endpoints = append(s.data.Endpoints, endpoint)
			values = append(values, endpoint)
		} else if enable && !endpoint.AutoConnect {
			endpoint.AutoConnect = true
			values = append(values, endpoint)
		}
	}
	return values
}
func (s *Service) ConfirmLocal() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.localState != "ready" {
		return problem("local_scanning", "Local agent detection is still running.", 409)
	}
	if s.data.LocalSetup.Confirmed {
		return nil
	}
	var available bool
	for _, agent := range s.localAgents {
		available = available || agent.State == "available"
	}
	if !available {
		return problem("local_missing", "No compatible local agents were found. Install Codex or Claude Code and detect again.", 409)
	}
	s.data.LocalSetup.Confirmed = true
	values := append(s.addLocalEndpointsLocked(true), s.data.LocalSetup)
	if err := s.commitLocked("local.updated", "", values...); err != nil {
		return err
	}
	for _, endpoint := range s.data.Endpoints {
		if endpoint.Local && endpoint.AutoConnect {
			s.connectLocked(endpoint)
		}
	}
	return nil
}
func (s *Service) localAgentLocked(provider string) (local.Agent, bool) {
	for _, agent := range s.localAgents {
		if agent.Provider == provider && agent.State == "available" {
			return agent, true
		}
	}
	return local.Agent{}, false
}

type Directory struct {
	Name string `json:"name"`
	Path string `json:"path"`
}
type DirectoryListing struct {
	Path        string      `json:"path"`
	Parent      string      `json:"parent"`
	Directories []Directory `json:"directories"`
	Limited     bool        `json:"limited"`
}

func (s *Service) Directories(path string) (DirectoryListing, error) {
	s.mu.Lock()
	confirmed := s.data.LocalSetup.Confirmed
	fallback := s.defaultDirectory
	s.mu.Unlock()
	if !confirmed {
		return DirectoryListing{}, problem("local_unconfirmed", "Confirm local agents before browsing project directories.", 409)
	}
	if path == "" {
		path = fallback
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return DirectoryListing{}, problem("local_directory", "The project directory is unavailable.", 400)
	}
	entries, err := os.ReadDir(absolute)
	if err != nil {
		return DirectoryListing{}, problem("local_directory", "The project directory is unavailable.", 400)
	}
	result := DirectoryListing{Path: absolute, Parent: filepath.Dir(absolute), Directories: []Directory{}}
	for _, entry := range entries {
		target := filepath.Join(absolute, entry.Name())
		directory := entry.IsDir()
		if entry.Type()&os.ModeSymlink != 0 {
			info, err := os.Stat(target)
			directory = err == nil && info.IsDir()
		}
		if directory {
			if len(result.Directories) >= 1000 {
				result.Limited = true
				break
			}
			result.Directories = append(result.Directories, Directory{Name: entry.Name(), Path: target})
		}
	}
	sort.Slice(result.Directories, func(i, j int) bool { return result.Directories[i].Name < result.Directories[j].Name })
	return result, nil
}
