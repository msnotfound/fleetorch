package main

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"

	"github.com/msnotfound/fleetorch/internal/config"
)

//go:embed builtin-presets/*.toml
var builtinPresets embed.FS

type preset struct {
	Name        string        `toml:"name"`
	Description string        `toml:"description"`
	Agents      []presetAgent `toml:"agent"`
}

type presetAgent struct {
	Type         string `toml:"type"`
	TaskIDSuffix string `toml:"task_id_suffix"`
	Prompt       string `toml:"prompt"`
}

func newPresetCmdReal() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "preset",
		Short: "Run coordinated agent teams from TOML blueprints",
	}
	cmd.AddCommand(newPresetListCmd(), newPresetRunCmd())
	return cmd
}

func newPresetListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed team presets",
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := config.Resolve()
			if err != nil {
				return err
			}
			if err := paths.EnsureDirs(); err != nil {
				return err
			}
			if err := seedBuiltinPresets(paths.PresetsDir); err != nil {
				return err
			}

			presets, err := loadPresets(paths.PresetsDir)
			if err != nil {
				return err
			}
			if len(presets) == 0 {
				fmt.Println("no presets installed")
				return nil
			}
			fmt.Printf("%-18s %s\n", "NAME", "DESCRIPTION")
			for _, p := range presets {
				fmt.Printf("%-18s %s\n", p.Name, p.Description)
			}
			return nil
		},
	}
}

func newPresetRunCmd() *cobra.Command {
	var (
		taskIDPrefix string
		promptVars   []string
	)
	cmd := &cobra.Command{
		Use:   "run <name>",
		Short: "Spawn every agent in a named team preset",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskIDPrefix = strings.TrimSpace(taskIDPrefix)
			if taskIDPrefix == "" {
				return errors.New("--task-id-prefix is required")
			}
			vars, err := parsePromptVars(promptVars)
			if err != nil {
				return err
			}

			paths, err := config.Resolve()
			if err != nil {
				return err
			}
			if err := paths.EnsureDirs(); err != nil {
				return err
			}
			if err := seedBuiltinPresets(paths.PresetsDir); err != nil {
				return err
			}
			p, err := loadPreset(paths.PresetsDir, args[0])
			if err != nil {
				return err
			}

			for _, agent := range p.Agents {
				taskID := taskIDPrefix + "-" + agent.TaskIDSuffix
				prompt, err := expandPresetPrompt(p.Name, agent.Prompt, vars)
				if err != nil {
					return fmt.Errorf("%s/%s: %w", p.Name, agent.TaskIDSuffix, err)
				}
				if err := doSpawn(agent.Type, taskID, prompt, "", 0, 0, "", false, false, ""); err != nil {
					return fmt.Errorf("spawn %s: %w", taskID, err)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&taskIDPrefix, "task-id-prefix", "", "Prefix for spawned task IDs")
	cmd.Flags().StringArrayVar(&promptVars, "prompt-var", nil, "Prompt template variable as key=value (repeatable)")
	return cmd
}

func seedBuiltinPresets(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir presets dir: %w", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read presets dir: %w", err)
	}
	if len(entries) > 0 {
		return nil
	}

	return fs.WalkDir(builtinPresets, "builtin-presets", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".toml" {
			return nil
		}
		contents, err := builtinPresets.ReadFile(path)
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

func loadPresets(dir string) ([]preset, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read presets dir: %w", err)
	}

	presets := make([]preset, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".toml" {
			continue
		}
		p, err := decodePresetFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping invalid preset file %s: %v\n", filepath.Join(dir, entry.Name()), err)
			continue
		}
		presets = append(presets, p)
	}
	sort.Slice(presets, func(i, j int) bool {
		return presets[i].Name < presets[j].Name
	})
	return presets, nil
}

func loadPreset(dir, name string) (*preset, error) {
	presets, err := loadPresets(dir)
	if err != nil {
		return nil, err
	}
	for _, p := range presets {
		if p.Name == name {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("unknown preset: %s", name)
}

func decodePresetFile(path string) (preset, error) {
	var p preset
	if _, err := toml.DecodeFile(path, &p); err != nil {
		return preset{}, err
	}
	p.Name = strings.TrimSpace(p.Name)
	p.Description = strings.TrimSpace(p.Description)
	if p.Name == "" {
		return preset{}, errors.New("name is required")
	}
	if len(p.Agents) == 0 {
		return preset{}, errors.New("at least one agent is required")
	}
	for i := range p.Agents {
		p.Agents[i].Type = strings.TrimSpace(p.Agents[i].Type)
		p.Agents[i].TaskIDSuffix = strings.TrimSpace(p.Agents[i].TaskIDSuffix)
		if p.Agents[i].Type == "" || p.Agents[i].TaskIDSuffix == "" || p.Agents[i].Prompt == "" {
			return preset{}, fmt.Errorf("agent %d requires type, task_id_suffix, and prompt", i+1)
		}
	}
	return p, nil
}

func parsePromptVars(values []string) (map[string]string, error) {
	vars := make(map[string]string, len(values))
	for _, value := range values {
		key, val, ok := strings.Cut(value, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("invalid --prompt-var %q, expected key=value", value)
		}
		vars[key] = val
	}
	return vars, nil
}

func expandPresetPrompt(name, prompt string, vars map[string]string) (string, error) {
	tmpl, err := template.New(name).Option("missingkey=error").Parse(prompt)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, vars); err != nil {
		return "", err
	}
	return out.String(), nil
}
