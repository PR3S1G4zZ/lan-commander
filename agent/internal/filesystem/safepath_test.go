package filesystem

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestSafePathRejectsTraversalElementsButAllowsDoubleDotsInNames(t *testing.T) {
	dir := t.TempDir()

	for _, bad := range []string{
		dir + string(filepath.Separator) + ".." + string(filepath.Separator) + "x",
		dir + "/../x",
		dir + "\\..\\x",
		"..",
		"../x",
	} {
		if _, err := safePath(bad); !errors.Is(err, ErrPathTraversal) {
			t.Errorf("safePath(%q) error = %v, want ErrPathTraversal", bad, err)
		}
	}

	for _, ok := range []string{
		filepath.Join(dir, "backup..old.txt"),
		filepath.Join(dir, "..hidden"),
		filepath.Join(dir, "name.."),
	} {
		if _, err := safePath(ok); err != nil {
			t.Errorf("safePath(%q) rejected a legitimate name: %v", ok, err)
		}
	}
}
