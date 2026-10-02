package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func makeFiles(t *testing.T, dir string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%04d.txt", i)), []byte("x"), 0o600); err != nil {
			t.Fatalf("create fixture: %v", err)
		}
	}
}

func TestListDirReportsRealTotalAndFirstPageByDefault(t *testing.T) {
	dir := t.TempDir()
	makeFiles(t, dir, 120)

	page, err := ListDir(dir, 0, 0)
	if err != nil {
		t.Fatalf("ListDir: %v", err)
	}
	if len(page.Entries) != DefaultDirEntries {
		t.Fatalf("entries = %d, want default page of %d", len(page.Entries), DefaultDirEntries)
	}
	if page.Total != 120 {
		t.Fatalf("Total = %d, want the real entry count 120", page.Total)
	}
	if !page.HasMore || page.NextOffset != DefaultDirEntries {
		t.Fatalf("HasMore=%v NextOffset=%d, want true/%d", page.HasMore, page.NextOffset, DefaultDirEntries)
	}
}

func TestListDirPagesCoverEveryEntryExactlyOnce(t *testing.T) {
	dir := t.TempDir()
	makeFiles(t, dir, 250)

	seen := map[string]int{}
	offset := 0
	for pages := 0; ; pages++ {
		if pages > 10 {
			t.Fatal("pagination did not terminate")
		}
		page, err := ListDir(dir, offset, 100)
		if err != nil {
			t.Fatalf("ListDir(offset=%d): %v", offset, err)
		}
		for _, e := range page.Entries {
			seen[e.Name]++
		}
		if !page.HasMore {
			break
		}
		offset = page.NextOffset
	}

	if len(seen) != 250 {
		t.Fatalf("saw %d distinct entries, want 250", len(seen))
	}
	for name, count := range seen {
		if count != 1 {
			t.Errorf("%s delivered %d times", name, count)
		}
	}
}

func TestListDirCapsLimitAndHandlesOffsetPastEnd(t *testing.T) {
	dir := t.TempDir()
	makeFiles(t, dir, 5)

	page, err := ListDir(dir, 0, MaxDirEntries*10)
	if err != nil || len(page.Entries) != 5 || page.HasMore {
		t.Fatalf("oversized limit: entries=%d hasMore=%v err=%v", len(page.Entries), page.HasMore, err)
	}

	page, err = ListDir(dir, 99, 10)
	if err != nil {
		t.Fatalf("offset past end should return an empty page, got error %v", err)
	}
	if len(page.Entries) != 0 || page.HasMore || page.Total != 5 {
		t.Fatalf("past-end page = %+v", page)
	}

	if _, err := ListDir(dir, -1, 10); err == nil {
		t.Fatal("negative offset must be rejected")
	}
}
