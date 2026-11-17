// SPDX-License-Identifier: GPL-2.0-or-later

package shellinit

import (
	"os/exec"
	"testing"
)

func TestScriptsParse(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		script, err := Script(shell)
		if err != nil {
			t.Fatal(err)
		}
		path, err := exec.LookPath(shell)
		if err != nil {
			continue
		}
		// -n checks syntax without running anything.
		if out, err := exec.Command(path, "-n", "-c", script).CombinedOutput(); err != nil {
			t.Errorf("%s: %v\n%s", shell, err, out)
		}
	}
	if _, err := Script("csh"); err == nil {
		t.Error("expected error for csh")
	}
}
