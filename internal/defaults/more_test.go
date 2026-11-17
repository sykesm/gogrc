// SPDX-License-Identifier: GPL-2.0-or-later

package defaults

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteErrors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Write(filepath.Join(file, "sub"), false); err == nil {
		t.Error("Write under a file succeeded")
	}

	dir := t.TempDir()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	if _, _, err := Write(dir, true); err == nil {
		t.Error("Write to a read-only directory succeeded")
	}
	// Without force, existence is checked first; an unreadable directory
	// makes that check fail.
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Write(dir, false); err == nil {
		t.Error("Write to an unreadable directory succeeded")
	}
}

func TestNamesAreSorted(t *testing.T) {
	names := Names()
	for i := 1; i < len(names); i++ {
		if names[i-1] >= names[i] {
			t.Fatalf("names not sorted at %d: %q, %q", i, names[i-1], names[i])
		}
	}
}
