// Package config resolves fleetorch's on-disk paths and loads the user config.
// Cross-platform paths follow OS conventions via os.UserConfigDir / UserCacheDir,
// with a $FLEETORCH_HOME escape hatch for testing and portable installs.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/BurntSushi/toml"
)

// Paths holds the resolved directory layout for this OS + user.
type Paths struct {
	ConfigDir   string // user config root (TOML files)
	DataDir     string // data root (state, logs, worktrees)
	AgentsDir   string // ConfigDir/agents (agent-type TOMLs)
	PresetsDir  string // ConfigDir/presets (team preset TOMLs)
	WorktreeDir string // DataDir/worktrees
	LogDir      string // DataDir/logs
	SocketDir   string // DataDir/sockets (per-task control sockets)
	StateFile   string // DataDir/state.json
	ConfigFile  string // ConfigDir/config.toml
}

// Resolve returns the active Paths. Honors $FLEETORCH_HOME if set
// (puts everything under one directory — handy for tests and portable mode).
func Resolve() (Paths, error) {
	if home := os.Getenv("FLEETORCH_HOME"); home != "" {
		return layoutUnder(home, home), nil
	}

	configBase, err := os.UserConfigDir()
	if err != nil {
		return Paths{}, fmt.Errorf("resolve user config dir: %w", err)
	}
	dataBase, err := userDataDir()
	if err != nil {
		return Paths{}, fmt.Errorf("resolve user data dir: %w", err)
	}

	return layoutUnder(
		filepath.Join(configBase, "fleetorch"),
		filepath.Join(dataBase, "fleetorch"),
	), nil
}

func layoutUnder(configRoot, dataRoot string) Paths {
	return Paths{
		ConfigDir:   configRoot,
		DataDir:     dataRoot,
		AgentsDir:   filepath.Join(configRoot, "agents"),
		PresetsDir:  filepath.Join(configRoot, "presets"),
		WorktreeDir: filepath.Join(dataRoot, "worktrees"),
		LogDir:      filepath.Join(dataRoot, "logs"),
		SocketDir:   filepath.Join(dataRoot, "sockets"),
		StateFile:   filepath.Join(dataRoot, "state.json"),
		ConfigFile:  filepath.Join(configRoot, "config.toml"),
	}
}

// userDataDir returns the per-user data directory for this OS.
// os.UserConfigDir covers config, but Go doesn't have a stdlib helper for data;
// we follow XDG / Apple / Microsoft conventions explicitly.
func userDataDir() (string, error) {
	switch runtime.GOOS {
	case "windows":
		if v := os.Getenv("LOCALAPPDATA"); v != "" {
			return v, nil
		}
		return "", errors.New("LOCALAPPDATA not set")
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support"), nil
	default: // linux, bsd, etc.
		if v := os.Getenv("XDG_DATA_HOME"); v != "" {
			return v, nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".local", "share"), nil
	}
}

// Policy holds concurrency and spend caps read from the [policy] TOML section.
// Zero values mean unlimited.
type Policy struct {
	MaxConcurrentTotal    int     `toml:"max_concurrent_total"`
	MaxConcurrentPerAgent int     `toml:"max_concurrent_per_agent"`
	MaxSpendUSDPerHour    float64 `toml:"max_spend_usd_per_hour"`
	MaxSpendUSDPerDay     float64 `toml:"max_spend_usd_per_day"`
}

// UserConfig is the parsed content of config.toml.
type UserConfig struct {
	Policy Policy `toml:"policy"`
}

// LoadConfig reads and parses config.toml. Returns zero-value defaults when
// the file does not exist (all caps unlimited).
func (p Paths) LoadConfig() (UserConfig, error) {
	var cfg UserConfig
	if _, err := os.Stat(p.ConfigFile); os.IsNotExist(err) {
		return cfg, nil
	}
	if _, err := toml.DecodeFile(p.ConfigFile, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

// EnsureDirs creates all required directories with 0o755 permissions.
// Safe to call repeatedly.
func (p Paths) EnsureDirs() error {
	for _, d := range []string{p.ConfigDir, p.AgentsDir, p.PresetsDir, p.DataDir, p.WorktreeDir, p.LogDir, p.SocketDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	return nil
}
