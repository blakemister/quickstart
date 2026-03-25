package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanupOldBinaries(t *testing.T) {
	dir := t.TempDir()

	// Create files: two old binaries, the current binary, and an unrelated file
	for _, name := range []string{
		"qs-old-20260101120000.exe",
		"qs-old-20260312093000.exe",
		"qs.exe",
		"other.exe",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	cleanupOldBinariesIn(dir)

	// qs-old-* should be deleted
	for _, name := range []string{"qs-old-20260101120000.exe", "qs-old-20260312093000.exe"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("expected %s to be deleted", name)
		}
	}

	// qs.exe and other.exe should remain
	for _, name := range []string{"qs.exe", "other.exe"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("expected %s to still exist", name)
		}
	}
}
