package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"tron"
)

type PluginDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
	Timeout     int                    `json:"timeout,omitempty"`
	Enabled     bool                   `json:"enabled,omitempty"`
}

type Plugin struct {
	Definition PluginDefinition
	Executable string
	Dir        string
}

type InternalTool interface {
	Definition() tron.Tool
	Execute(argsJSON string) (string, error)
}

type ContextAwareTool interface {
	InternalTool
	SetContext(chatID string)
}

type Manager struct {
	plugins       map[string]*Plugin
	internalTools map[string]InternalTool
	debug         bool
}

func NewManager(pluginDir string, debug bool) (*Manager, error) {
	m := &Manager{
		plugins:       make(map[string]*Plugin),
		internalTools: make(map[string]InternalTool),
		debug:         debug,
	}

	if err := m.loadPlugins(pluginDir); err != nil {
		return nil, err
	}

	return m, nil
}

func (m *Manager) RegisterTool(name string, tool InternalTool) {
	m.internalTools[name] = tool
	if m.debug {
		fmt.Printf("[plugin] registered internal tool: %s\n", name)
	}
}

func (m *Manager) loadPlugins(pluginDir string) error {
	absPluginDir, err := filepath.Abs(pluginDir)
	if err != nil {
		return fmt.Errorf("abs path: %w", err)
	}

	entries, err := os.ReadDir(absPluginDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read plugin dir: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pluginPath := filepath.Join(absPluginDir, entry.Name())
		plugin, err := m.loadPlugin(pluginPath)
		if err != nil {
			if m.debug {
				fmt.Printf("[plugin] skip %s: %v\n", entry.Name(), err)
			}
			continue
		}

		if plugin.Definition.Enabled {
			m.plugins[plugin.Definition.Name] = plugin
			if m.debug {
				fmt.Printf("[plugin] loaded: %s\n", plugin.Definition.Name)
			}
		}
	}

	return nil
}

func (m *Manager) loadPlugin(dir string) (*Plugin, error) {
	defPath := filepath.Join(dir, "definition.json")
	data, err := os.ReadFile(defPath)
	if err != nil {
		return nil, fmt.Errorf("read definition: %w", err)
	}

	var def PluginDefinition
	if err := json.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("parse definition: %w", err)
	}

	if def.Timeout == 0 {
		def.Timeout = 30
	}

	executable := m.findExecutable(dir)
	if executable == "" {
		return nil, fmt.Errorf("no executable found")
	}

	return &Plugin{
		Definition: def,
		Executable: executable,
		Dir:        dir,
	}, nil
}

func (m *Manager) findExecutable(dir string) string {
	candidates := []string{"run", "run.sh", "run.py", "run.rb", "main"}

	for _, name := range candidates {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.Mode()&0111 != 0 {
			return path
		}
	}

	return ""
}

func (m *Manager) ExecuteWithContext(name string, argsJSON string, chatID string) (string, error) {
	if tool, ok := m.internalTools[name]; ok {
		if ctxTool, ok := tool.(ContextAwareTool); ok {
			ctxTool.SetContext(chatID)
		}
		return tool.Execute(argsJSON)
	}

	plugin, ok := m.plugins[name]
	if !ok {
		return "", fmt.Errorf("unknown plugin: %s", name)
	}

	timeout := time.Duration(plugin.Definition.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, plugin.Executable)
	cmd.Dir = plugin.Dir
	cmd.Stdin = bytes.NewReader([]byte(argsJSON))

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("plugin timeout after %ds", plugin.Definition.Timeout)
	}
	if err != nil {
		errMsg := stderr.String()
		if errMsg == "" {
			errMsg = err.Error()
		}
		return "", fmt.Errorf("plugin error: %s", errMsg)
	}

	return stdout.String(), nil
}

func (m *Manager) Execute(name string, argsJSON string) (string, error) {
	if tool, ok := m.internalTools[name]; ok {
		return tool.Execute(argsJSON)
	}

	plugin, ok := m.plugins[name]
	if !ok {
		return "", fmt.Errorf("unknown plugin: %s", name)
	}

	timeout := time.Duration(plugin.Definition.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, plugin.Executable)
	cmd.Dir = plugin.Dir
	cmd.Stdin = bytes.NewReader([]byte(argsJSON))

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("plugin timeout after %ds", plugin.Definition.Timeout)
	}
	if err != nil {
		errMsg := stderr.String()
		if errMsg == "" {
			errMsg = err.Error()
		}
		return "", fmt.Errorf("plugin error: %s", errMsg)
	}

	return stdout.String(), nil
}

func (m *Manager) GetTools() []tron.Tool {
	var tools []tron.Tool

	for _, tool := range m.internalTools {
		tools = append(tools, tool.Definition())
	}

	for _, plugin := range m.plugins {
		tools = append(tools, tron.Tool{
			Type: "function",
			Function: tron.ToolFunction{
				Name:        plugin.Definition.Name,
				Description: plugin.Definition.Description,
				Parameters:  plugin.Definition.Parameters,
			},
		})
	}

	return tools
}

func (m *Manager) HasPlugin(name string) bool {
	if _, ok := m.internalTools[name]; ok {
		return true
	}
	_, ok := m.plugins[name]
	return ok
}

func (m *Manager) PluginCount() int {
	return len(m.plugins) + len(m.internalTools)
}
