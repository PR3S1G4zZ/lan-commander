package scripting

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
	"time"
)

// Script represents a saved script that can be executed on agents.
type Script struct {
	Name      string    `json:"name"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ExecutionResult holds the result of executing a single command within a script.
type ExecutionResult struct {
	LineNumber int    `json:"line_number"`
	Command    string `json:"command"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ExitCode   int    `json:"exit_code"`
	Duration   int    `json:"duration_ms"`
}

// ScriptResult holds the complete result of script execution.
type ScriptResult struct {
	ScriptName   string             `json:"script_name"`
	AgentID      string             `json:"agent_id"`
	Results      []ExecutionResult  `json:"results"`
	TotalLines   int                `json:"total_lines"`
	SuccessCount int                `json:"success_count"`
	FailedCount  int                `json:"failed_count"`
}

// Engine manages scripts and can execute them on agents.
type Engine struct {
	scripts  map[string]*Script
	mu       sync.RWMutex
	basePath string
}

// NewEngine creates a new script engine, loading scripts from the scripts directory.
func NewEngine() *Engine {
	e := &Engine{
		scripts:  make(map[string]*Script),
		basePath: "scripts",
	}

	// Attempt to load scripts from disk (non-fatal if directory doesn't exist)
	if err := e.loadFromDisk(); err != nil {
		log.Printf("scripting: no scripts loaded from disk: %v", err)
	}

	return e
}

// NewEngineWithPath creates a new script engine with a custom scripts directory.
func NewEngineWithPath(path string) *Engine {
	e := &Engine{
		scripts:  make(map[string]*Script),
		basePath: path,
	}

	if err := e.loadFromDisk(); err != nil {
		log.Printf("scripting: no scripts loaded from disk: %v", err)
	}

	return e
}

// Save stores a script both in memory and on disk.
func (e *Engine) Save(name string, content string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("script name cannot be empty")
	}
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("script content cannot be empty")
	}

	now := time.Now()

	e.mu.Lock()
	existing, exists := e.scripts[name]
	if exists {
		existing.Content = content
		existing.UpdatedAt = now
	} else {
		e.scripts[name] = &Script{
			Name:      name,
			Content:   content,
			CreatedAt: now,
			UpdatedAt: now,
		}
	}
	e.mu.Unlock()

	// Persist to disk
	if err := e.saveToDisk(name); err != nil {
		return fmt.Errorf("failed to save script to disk: %w", err)
	}

	return nil
}

// Get retrieves a script by name.
func (e *Engine) Get(name string) (*Script, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	script, exists := e.scripts[name]
	if !exists {
		return nil, fmt.Errorf("script %q not found", name)
	}
	return script, nil
}

// List returns all available scripts.
func (e *Engine) List() []Script {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]Script, 0, len(e.scripts))
	for _, s := range e.scripts {
		result = append(result, *s)
	}
	return result
}

// Delete removes a script by name.
func (e *Engine) Delete(name string) error {
	e.mu.Lock()
	_, exists := e.scripts[name]
	if !exists {
		e.mu.Unlock()
		return fmt.Errorf("script %q not found", name)
	}
	delete(e.scripts, name)
	e.mu.Unlock()

	// Remove from disk
	path := filepath.Join(e.basePath, name+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete script file: %w", err)
	}

	return nil
}

// Execute run a script as a series of commands, returning combined results.
// It splits the script content by newlines, processes each line as a command,
// variables ({{VAR}}) are evaluated before execution.
func (e *Engine) Execute(script *Script, vars map[string]string, executor CommandExecutor) (*ScriptResult, error) {
	if script == nil {
		return nil, fmt.Errorf("script cannot be nil")
	}
	if executor == nil {
		return nil, fmt.Errorf("command executor cannot be nil")
	}

	lines := strings.Split(strings.TrimSpace(script.Content), "\n")
	result := &ScriptResult{
		ScriptName: script.Name,
		Results:    make([]ExecutionResult, 0, len(lines)),
		TotalLines: len(lines),
	}

	for i, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}

		// Process variables
		processedCommand, err := e.processVariables(line, vars)
		if err != nil {
			result.Results = append(result.Results, ExecutionResult{
				LineNumber: i + 1,
				Command:    line,
				Stderr:     fmt.Sprintf("variable processing error: %v", err),
				ExitCode:   1,
			})
			result.FailedCount++
			continue
		}

		// Execute the command
		execResult, err := executor(processedCommand)
		if err != nil {
			result.Results = append(result.Results, ExecutionResult{
				LineNumber: i + 1,
				Command:    processedCommand,
				Stderr:     err.Error(),
				ExitCode:   1,
			})
			result.FailedCount++
			continue
		}

		execResult.LineNumber = i + 1
		execResult.Command = processedCommand
		result.Results = append(result.Results, *execResult)

		if execResult.ExitCode == 0 {
			result.SuccessCount++
		} else {
			result.FailedCount++
		}
	}

	return result, nil
}

// CommandExecutor is a function that executes a single command string and returns the result.
type CommandExecutor func(command string) (*ExecutionResult, error)

// processVariables replaces {{VAR}} placeholders in the command string with provided variable values.
func (e *Engine) processVariables(command string, vars map[string]string) (string, error) {
	if vars == nil {
		return command, nil
	}

	// Build template with function map
	funcMap := template.FuncMap{
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
		"trim":  strings.TrimSpace,
	}

	tmpl, err := template.New("cmd").Funcs(funcMap).Parse(command)
	if err != nil {
		return "", fmt.Errorf("failed to parse command template: %w", err)
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("failed to execute command template: %w", err)
	}

	return buf.String(), nil
}

// --- disk persistence ---

func (e *Engine) loadFromDisk() error {
	// Ensure directory exists
	if err := os.MkdirAll(e.basePath, 0755); err != nil {
		return fmt.Errorf("cannot create scripts directory: %w", err)
	}

	entries, err := os.ReadDir(e.basePath)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		path := filepath.Join(e.basePath, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("scripting: failed to read %s: %v", path, err)
			continue
		}

		var script Script
		if err := json.Unmarshal(data, &script); err != nil {
			log.Printf("scripting: failed to parse %s: %v", path, err)
			continue
		}

		e.scripts[script.Name] = &script
	}

	return nil
}

func (e *Engine) saveToDisk(name string) error {
	// Ensure directory exists
	if err := os.MkdirAll(e.basePath, 0755); err != nil {
		return err
	}

	e.mu.RLock()
	script, exists := e.scripts[name]
	e.mu.RUnlock()

	if !exists {
		return fmt.Errorf("script %q not found in memory", name)
	}

	data, err := json.MarshalIndent(script, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal script: %w", err)
	}

	path := filepath.Join(e.basePath, name+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write script file: %w", err)
	}

	return nil
}
