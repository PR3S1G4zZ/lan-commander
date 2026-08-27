package scripting

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

func TestEngineSavePersistsAndReloadsScripts(t *testing.T) {
	dir := t.TempDir()
	engine := NewEngineWithPath(dir)

	if err := engine.Save("hello", "echo hello"); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "hello.json"))
	if err != nil {
		t.Fatalf("read persisted script: %v", err)
	}
	var onDisk Script
	if err := json.Unmarshal(data, &onDisk); err != nil {
		t.Fatalf("decode persisted script: %v", err)
	}
	if onDisk.Name != "hello" || onDisk.Content != "echo hello" {
		t.Fatalf("persisted script = %#v, want name/content hello/echo hello", onDisk)
	}
	if onDisk.CreatedAt.IsZero() || onDisk.UpdatedAt.IsZero() {
		t.Fatalf("persisted timestamps are not populated: %#v", onDisk)
	}

	reloaded := NewEngineWithPath(dir)
	got, err := reloaded.Get("hello")
	if err != nil {
		t.Fatalf("reloaded Get() error = %v", err)
	}
	if got.Name != onDisk.Name || got.Content != onDisk.Content {
		t.Fatalf("reloaded script = %#v, want %#v", got, onDisk)
	}
	if !got.CreatedAt.Equal(onDisk.CreatedAt) || !got.UpdatedAt.Equal(onDisk.UpdatedAt) {
		t.Fatalf("reloaded timestamps = %#v, want %#v", got, onDisk)
	}
}

func TestEngineSaveUpdatesExistingScriptAndPreservesCreationTime(t *testing.T) {
	engine := NewEngineWithPath(t.TempDir())
	if err := engine.Save("demo", "first"); err != nil {
		t.Fatalf("initial Save() error = %v", err)
	}
	first, err := engine.Get("demo")
	if err != nil {
		t.Fatalf("initial Get() error = %v", err)
	}
	createdAt := first.CreatedAt
	updatedAt := first.UpdatedAt

	if err := engine.Save("demo", "second"); err != nil {
		t.Fatalf("update Save() error = %v", err)
	}
	updated, err := engine.Get("demo")
	if err != nil {
		t.Fatalf("updated Get() error = %v", err)
	}
	if updated.Content != "second" {
		t.Fatalf("updated content = %q, want %q", updated.Content, "second")
	}
	if !updated.CreatedAt.Equal(createdAt) {
		t.Fatalf("updated CreatedAt = %v, want unchanged %v", updated.CreatedAt, createdAt)
	}
	if updated.UpdatedAt.Before(updatedAt) {
		t.Fatalf("updated UpdatedAt = %v, went backwards from %v", updated.UpdatedAt, updatedAt)
	}

	reloaded := NewEngineWithPath(engine.basePath)
	persisted, err := reloaded.Get("demo")
	if err != nil {
		t.Fatalf("reloaded updated Get() error = %v", err)
	}
	if persisted.Content != "second" || !persisted.CreatedAt.Equal(createdAt) {
		t.Fatalf("reloaded updated script = %#v, want updated content and original creation time", persisted)
	}
}

func TestEngineDeleteRemovesScriptFromMemoryAndDisk(t *testing.T) {
	dir := t.TempDir()
	engine := NewEngineWithPath(dir)
	if err := engine.Save("remove-me", "echo remove"); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	if err := engine.Delete("remove-me"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "remove-me.json")); !os.IsNotExist(err) {
		t.Fatalf("script file stat error = %v, want not-exist", err)
	}
	if _, err := engine.Get("remove-me"); err == nil {
		t.Fatal("Get() succeeded after Delete()")
	}
	if err := engine.Delete("remove-me"); err == nil {
		t.Fatal("second Delete() succeeded for a missing script")
	}
}

func TestEngineRejectsBlankNamesAndContent(t *testing.T) {
	engine := NewEngineWithPath(t.TempDir())
	cases := []struct {
		name    string
		content string
	}{
		{name: "", content: "echo ok"},
		{name: "   ", content: "echo ok"},
		{name: "demo", content: ""},
		{name: "demo", content: " \n\t "},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("name=%q_content=%q", tc.name, tc.content), func(t *testing.T) {
			if err := engine.Save(tc.name, tc.content); err == nil {
				t.Fatalf("Save(%q, %q) succeeded for blank input", tc.name, tc.content)
			}
		})
	}
	if got := engine.List(); len(got) != 0 {
		t.Fatalf("List() after rejected saves = %#v, want empty", got)
	}
}

func TestEngineRejectsScriptNameThatEscapesBasePath(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(filepath.Dir(dir), filepath.Base(dir)+"-escape.json")
	if _, err := os.Stat(outside); err == nil {
		t.Fatalf("escape fixture already exists: %s", outside)
	} else if !os.IsNotExist(err) {
		t.Fatalf("check escape fixture: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(outside) })

	engine := NewEngineWithPath(dir)
	name := filepath.Join("..", filepath.Base(dir)+"-escape")
	if err := engine.Save(name, "echo unsafe"); err == nil {
		t.Errorf("Save(%q) succeeded for a path-escaping script name", name)
	}
	if _, err := os.Stat(outside); err == nil {
		t.Fatalf("path-escaping script was persisted outside base path: %s", outside)
	} else if !os.IsNotExist(err) {
		t.Fatalf("check escaped script path: %v", err)
	}
}

func TestEngineRejectsPathEscapingScriptNameOnDelete(t *testing.T) {
	dir := t.TempDir()
	escapedName := filepath.Join("..", filepath.Base(dir)+"-delete-escape")
	escapedPath := filepath.Join(filepath.Dir(dir), filepath.Base(dir)+"-delete-escape.json")
	if err := os.WriteFile(escapedPath, []byte("must remain"), 0600); err != nil {
		t.Fatalf("create escaped delete fixture: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(escapedPath) })

	engine := NewEngineWithPath(dir)
	engine.scripts[escapedName] = &Script{Name: escapedName, Content: "echo unsafe"}
	if err := engine.Delete(escapedName); err == nil {
		t.Errorf("Delete(%q) succeeded for a path-escaping script name", escapedName)
	}
	if _, err := os.Stat(escapedPath); err != nil {
		t.Fatalf("escaped delete fixture was removed or became inaccessible: %v", err)
	}
}

func TestEngineLoadSkipsMalformedAndNonJSONFiles(t *testing.T) {
	dir := t.TempDir()
	valid := Script{Name: "loaded", Content: "echo loaded"}
	data, err := json.Marshal(valid)
	if err != nil {
		t.Fatalf("marshal valid fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "loaded.json"), data, 0600); err != nil {
		t.Fatalf("write valid fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{"), 0600); err != nil {
		t.Fatalf("write malformed fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("not a script"), 0600); err != nil {
		t.Fatalf("write non-JSON fixture: %v", err)
	}

	engine := NewEngineWithPath(dir)
	if _, err := engine.Get("loaded"); err != nil {
		t.Fatalf("Get() for valid fixture error = %v", err)
	}
	if _, err := engine.Get("broken"); err == nil {
		t.Fatal("Get() succeeded for malformed JSON fixture")
	}
	if _, err := engine.Get("notes"); err == nil {
		t.Fatal("Get() succeeded for non-JSON fixture")
	}
}

func TestExecuteExpandsVariablesAndTemplateFunctions(t *testing.T) {
	engine := NewEngineWithPath(t.TempDir())
	script := &Script{
		Name:    "variables",
		Content: `echo {{.NAME}}|{{upper .NAME}}|{{lower .NAME}}|{{trim .PAD}}|{{.SPECIAL}}`,
	}
	vars := map[string]string{
		"NAME":    "MiXeD",
		"PAD":     "  padded value  ",
		"SPECIAL": `$(echo injected) && "quoted"`,
	}
	var commands []string
	result, err := engine.Execute(script, vars, func(command string) (*ExecutionResult, error) {
		commands = append(commands, command)
		return &ExecutionResult{Stdout: "ok", ExitCode: 0}, nil
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	wantCommand := `echo MiXeD|MIXED|mixed|padded value|$(echo injected) && "quoted"`
	if len(commands) != 1 || commands[0] != wantCommand {
		t.Fatalf("executor commands = %#v, want [%q]", commands, wantCommand)
	}
	if result.SuccessCount != 1 || result.FailedCount != 0 || len(result.Results) != 1 {
		t.Fatalf("execution summary = %#v, want one success", result)
	}
	if result.Results[0].Command != wantCommand || result.Results[0].LineNumber != 1 {
		t.Fatalf("execution result = %#v, want processed command on line 1", result.Results[0])
	}
}

func TestExecuteRecordsTemplateErrorsPerLineAndContinues(t *testing.T) {
	engine := NewEngineWithPath(t.TempDir())
	script := &Script{
		Name:    "template-errors",
		Content: "bad {{\n{{index .NAME \"key\"}}\nok",
	}
	var commands []string
	result, err := engine.Execute(script, map[string]string{"NAME": "value"}, func(command string) (*ExecutionResult, error) {
		commands = append(commands, command)
		return &ExecutionResult{ExitCode: 0}, nil
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(commands) != 1 || commands[0] != "ok" {
		t.Fatalf("executor commands = %#v, want only final valid command", commands)
	}
	if result.TotalLines != 3 || len(result.Results) != 3 {
		t.Fatalf("execution line accounting = total %d/results %d, want 3/3", result.TotalLines, len(result.Results))
	}
	if result.Results[0].ExitCode != 1 || !strings.Contains(result.Results[0].Stderr, "failed to parse command template") {
		t.Fatalf("parse-error result = %#v", result.Results[0])
	}
	if result.Results[1].ExitCode != 1 || !strings.Contains(result.Results[1].Stderr, "failed to execute command template") {
		t.Fatalf("execute-error result = %#v", result.Results[1])
	}
	if result.Results[2].ExitCode != 0 || result.Results[2].Command != "ok" {
		t.Fatalf("valid result after template errors = %#v", result.Results[2])
	}
	if result.SuccessCount != 1 || result.FailedCount != 2 {
		t.Fatalf("execution counts = success %d failed %d, want 1/2", result.SuccessCount, result.FailedCount)
	}
}

func TestExecuteRecordsExecutorErrorsAndNonzeroExitCodes(t *testing.T) {
	engine := NewEngineWithPath(t.TempDir())
	script := &Script{Name: "executor-errors", Content: "first\nsecond\nthird"}
	var commands []string
	result, err := engine.Execute(script, nil, func(command string) (*ExecutionResult, error) {
		commands = append(commands, command)
		switch command {
		case "first":
			return &ExecutionResult{Stdout: "ok", ExitCode: 0, Duration: 3}, nil
		case "second":
			return nil, errors.New("connection reset")
		case "third":
			return &ExecutionResult{Stderr: "remote failure", ExitCode: 7, Duration: 4}, nil
		default:
			return nil, errors.New("unexpected command")
		}
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !reflectStringSlicesEqual(commands, []string{"first", "second", "third"}) {
		t.Fatalf("executor commands = %#v", commands)
	}
	if result.TotalLines != 3 || len(result.Results) != 3 {
		t.Fatalf("execution summary = total %d/results %d, want 3/3", result.TotalLines, len(result.Results))
	}
	if result.Results[0].LineNumber != 1 || result.Results[0].Stdout != "ok" || result.Results[0].Duration != 3 {
		t.Fatalf("successful execution result = %#v", result.Results[0])
	}
	if result.Results[1].LineNumber != 2 || result.Results[1].ExitCode != 1 || result.Results[1].Stderr != "connection reset" {
		t.Fatalf("executor-error result = %#v", result.Results[1])
	}
	if result.Results[2].LineNumber != 3 || result.Results[2].ExitCode != 7 || result.Results[2].Stderr != "remote failure" {
		t.Fatalf("nonzero execution result = %#v", result.Results[2])
	}
	if result.SuccessCount != 1 || result.FailedCount != 2 {
		t.Fatalf("execution counts = success %d failed %d, want 1/2", result.SuccessCount, result.FailedCount)
	}
}

func TestExecuteRejectsNilScriptAndExecutor(t *testing.T) {
	engine := NewEngineWithPath(t.TempDir())
	if _, err := engine.Execute(nil, nil, func(string) (*ExecutionResult, error) { return nil, nil }); err == nil {
		t.Fatal("Execute(nil, ...) succeeded")
	}
	if _, err := engine.Execute(&Script{Name: "demo", Content: "echo demo"}, nil, nil); err == nil {
		t.Fatal("Execute(..., nil) succeeded")
	}
}

func TestExecutePreservesPhysicalLineNumbersAroundCommentsAndBlankLines(t *testing.T) {
	engine := NewEngineWithPath(t.TempDir())
	script := &Script{
		Name:    "line-numbers",
		Content: "\n  \n# ignored\n echo first \n// ignored\n\n echo second\n\n",
	}
	var commands []string
	result, err := engine.Execute(script, nil, func(command string) (*ExecutionResult, error) {
		commands = append(commands, command)
		return &ExecutionResult{ExitCode: 0}, nil
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !reflectStringSlicesEqual(commands, []string{"echo first", "echo second"}) {
		t.Fatalf("executor commands = %#v, want trimmed executable lines", commands)
	}
	if result.TotalLines != 9 {
		t.Errorf("TotalLines = %d, want physical line count 9", result.TotalLines)
	}
	if len(result.Results) != 2 {
		t.Fatalf("result count = %d, want 2", len(result.Results))
	}
	if result.Results[0].LineNumber != 4 || result.Results[1].LineNumber != 7 {
		t.Errorf("result line numbers = %d and %d, want 4 and 7", result.Results[0].LineNumber, result.Results[1].LineNumber)
	}
}

func TestEngineConcurrentSavesToDistinctScripts(t *testing.T) {
	engine := NewEngineWithPath(t.TempDir())
	const scriptCount = 24
	var wg sync.WaitGroup
	errs := make(chan error, scriptCount)
	for i := 0; i < scriptCount; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			name := fmt.Sprintf("script-%02d", i)
			if err := engine.Save(name, "echo "+name); err != nil {
				errs <- fmt.Errorf("Save(%q): %w", name, err)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}

	list := engine.List()
	if len(list) != scriptCount {
		t.Fatalf("List() count = %d, want %d", len(list), scriptCount)
	}
	names := make([]string, 0, len(list))
	for _, script := range list {
		names = append(names, script.Name)
	}
	sort.Strings(names)
	for i, name := range names {
		want := fmt.Sprintf("script-%02d", i)
		if name != want {
			t.Fatalf("sorted script name %d = %q, want %q", i, name, want)
		}
	}
}

func reflectStringSlicesEqual(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
