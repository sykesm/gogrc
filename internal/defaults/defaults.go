// SPDX-License-Identifier: GPL-2.0-or-later

// Package defaults embeds the stock grc configuration: grc.conf and the
// color files, taken from grc's newmaster branch (1.14rc1, commit 33942c1).
package defaults

import (
	"embed"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

//go:embed files
var files embed.FS

// FS returns the embedded configuration files.
func FS() fs.FS {
	sub, err := fs.Sub(files, "files")
	if err != nil {
		panic(err)
	}
	return sub
}

// Names returns the names of the embedded files, sorted.
func Names() []string {
	entries, err := fs.ReadDir(FS(), ".")
	if err != nil {
		panic(err)
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	sort.Strings(names)
	return names
}

// Write copies the embedded files into dir, creating it as needed. Existing
// files are left alone unless force is set.
func Write(dir string, force bool) (written, skipped []string, err error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, nil, err
	}
	for _, name := range Names() {
		path := filepath.Join(dir, name)
		if !force {
			if _, err := os.Lstat(path); err == nil {
				skipped = append(skipped, path)
				continue
			} else if !errors.Is(err, fs.ErrNotExist) {
				return written, skipped, err
			}
		}
		data, err := fs.ReadFile(FS(), name)
		if err != nil {
			return written, skipped, err
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return written, skipped, err
		}
		written = append(written, path)
	}
	return written, skipped, nil
}
