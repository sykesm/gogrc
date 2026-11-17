// SPDX-License-Identifier: GPL-2.0-or-later

package defaults

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWrite(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "gogrc", "defaults")
	written, skipped, err := Write(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != len(Names()) || len(skipped) != 0 {
		t.Fatalf("written %d, skipped %d, want %d and 0", len(written), len(skipped), len(Names()))
	}
	if _, err := os.Stat(filepath.Join(dir, "grc.conf")); err != nil {
		t.Error(err)
	}

	mine := filepath.Join(dir, "conf.ls")
	os.WriteFile(mine, []byte("edited"), 0o644)
	if _, skipped, _ = Write(dir, false); len(skipped) != len(Names()) {
		t.Errorf("second write skipped %d files, want all", len(skipped))
	}
	if data, _ := os.ReadFile(mine); string(data) != "edited" {
		t.Error("existing file overwritten without force")
	}
	Write(dir, true)
	if data, _ := os.ReadFile(mine); string(data) == "edited" {
		t.Error("force did not overwrite")
	}
}
