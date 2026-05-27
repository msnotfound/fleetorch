// Package agents loads fleetorch agent-type descriptors from TOML files.
package agents

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/msnotfound/fleetorch/internal/types"
)

var ErrUnknownAgent = errors.New("unknown agent type")

//go:embed examples/*.toml
var defaultAgents embed.FS

// AgentType is a local renderable view of types.AgentType.
//
// Registry stores shared types.AgentType values; this named type exists because
// Go only permits methods on types declared in the same package.
type AgentType types.AgentType

// Registry holds loaded agent types by name.
type Registry struct {
	agents map[string]*types.AgentType
}

// Load reads every *.toml file in dir and returns a registry of valid agents.
func Load(dir string) (*Registry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read agents dir: %w", err)
	}

	registry := &Registry{agents: make(map[string]*types.AgentType)}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".toml" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		var agent types.AgentType
		if _, err := toml.DecodeFile(path, &agent); err != nil {
			warnInvalid(path, err)
			continue
		}
		agent.Name = strings.TrimSpace(agent.Name)
		agent.Command = strings.TrimSpace(agent.Command)
		if agent.Name == "" || agent.Command == "" {
			warnInvalid(path, errors.New("name and command are required"))
			continue
		}

		agentCopy := agent
		registry.agents[agent.Name] = &agentCopy
	}

	return registry, nil
}

// Get returns an agent type by name.
func (r *Registry) Get(name string) (*types.AgentType, error) {
	if r == nil || r.agents == nil {
		return nil, fmt.Errorf("%w: %s", ErrUnknownAgent, name)
	}
	agent, ok := r.agents[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownAgent, name)
	}
	return agent, nil
}

// List returns all loaded agent types sorted by name.
func (r *Registry) List() []*types.AgentType {
	if r == nil || len(r.agents) == 0 {
		return nil
	}

	names := make([]string, 0, len(r.agents))
	for name := range r.agents {
		names = append(names, name)
	}
	sort.Strings(names)

	agents := make([]*types.AgentType, 0, len(names))
	for _, name := range names {
		agents = append(agents, r.agents[name])
	}
	return agents
}

// SeedDefaults writes embedded default agent TOMLs into dir if dir is empty.
func SeedDefaults(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir agents dir: %w", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read agents dir: %w", err)
	}
	if len(entries) > 0 {
		return nil
	}

	return fs.WalkDir(defaultAgents, "examples", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".toml" {
			return nil
		}

		contents, err := defaultAgents.ReadFile(path)
		if err != nil {
			return err
		}
		target := filepath.Join(dir, filepath.Base(path))
		if err := os.WriteFile(target, contents, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", target, err)
		}
		return nil
	})
}

// Render fills in {prompt} placeholders and returns command plus final args.
func (a *AgentType) Render(prompt string) []string {
	if a == nil {
		return nil
	}
	return renderAgent(types.AgentType(*a), prompt)
}

// Render fills in {prompt} placeholders for a shared agent type.
func Render(agent *types.AgentType, prompt string) []string {
	if agent == nil {
		return nil
	}
	return renderAgent(*agent, prompt)
}

func renderAgent(agent types.AgentType, prompt string) []string {
	argv := make([]string, 0, 1+len(agent.Args)+1)
	if agent.Command != "" {
		argv = append(argv, strings.ReplaceAll(agent.Command, "{prompt}", prompt))
	}
	for _, arg := range agent.Args {
		argv = append(argv, strings.ReplaceAll(arg, "{prompt}", prompt))
	}
	if agent.PromptArg != "" {
		argv = append(argv, strings.ReplaceAll(agent.PromptArg, "{prompt}", prompt))
	}
	return argv
}

func warnInvalid(path string, err error) {
	fmt.Fprintf(os.Stderr, "warning: skipping invalid agent file %s: %v\n", path, err)
}
