package scripting

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScriptNamesAreConfinedToScriptsDirectory(t *testing.T) {
	engine := NewEngineWithPath(t.TempDir())
	cases := []struct {
		name string
		bad  bool
	}{
		{name: "maintenance-v2"},
		{name: "daily report"},
		{name: "../outside", bad: true},
		{name: `..\outside`, bad: true},
		{name: `sub\script`, bad: true},
		{name: "sub/script", bad: true},
		{name: filepath.Join(string(os.PathSeparator), "absolute"), bad: true},
		{name: "..", bad: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := engine.Save(tc.name, "echo safe")
			if tc.bad {
				if err == nil {
					t.Fatal("unsafe name accepted")
				}
				if err := engine.Delete(tc.name); err == nil {
					t.Fatal("unsafe delete name accepted")
				}
				if _, err := engine.Get(tc.name); err == nil {
					t.Fatal("unsafe load name accepted")
				}
				return
			}
			if err != nil {
				t.Fatalf("legal name rejected: %v", err)
			}
			if _, err := engine.Get(tc.name); err != nil {
				t.Fatalf("saved script cannot be loaded: %v", err)
			}
			if err := engine.Delete(tc.name); err != nil {
				t.Fatalf("legal script cannot be deleted: %v", err)
			}
		})
	}
}

func TestLoadFromDiskIgnoresScriptSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte(`{"name":"outside","content":"echo"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link.json")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	engine := NewEngineWithPath(root)
	if _, err := engine.Get("outside"); err == nil {
		t.Fatal("loaded script from outside the scripts directory")
	}
}
