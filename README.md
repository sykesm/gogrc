# gogrc

A generic colorizer for command output, compatible with the configuration of
[grc](https://github.com/garabik/grc), and built as a single static Go binary.

```sh
gogrc ping -c 3 example.com
gogrc -es make
tail -f /var/log/system.log | gogrc --cat conf.log
```

gogrc reads the same `grc.conf` and `conf.*` color files as grc and colors
plain text identically: the test suite runs Python grcat and gogrc over a
corpus with every stock color file and requires byte-identical output. Unlike
grc, the command still sees a terminal, so it keeps its columns, progress
output and own colors (see [Differences from grc](#differences-from-grc)).

## Installation

With [Homebrew](https://brew.sh):

```sh
brew install sykesm/tap/gogrc
```

Or download a binary for macOS or Linux (amd64 or arm64) from the
[releases page](https://github.com/sykesm/gogrc/releases), unpack it and put
`gogrc` somewhere on your `PATH`:

```sh
tar xzf gogrc_*_darwin_arm64.tar.gz gogrc
install gogrc ~/.local/bin/
```

The binaries are not signed. If macOS refuses to run one downloaded with a
browser, clear the quarantine flag with `xattr -d com.apple.quarantine gogrc`.

Or install with Go 1.26 or later:

```sh
go install github.com/sykesm/gogrc@latest    # installs to $(go env GOPATH)/bin
```

Or build from a clone:

```sh
git clone https://github.com/sykesm/gogrc.git
cd gogrc
go install .
```

### Shell setup

To wrap common commands (`ls`, `ping`, `netstat`, `docker`, ...), add one of
these to your shell's startup file:

```sh
eval "$(gogrc --init zsh)"     # ~/.zshrc
eval "$(gogrc --init bash)"    # ~/.bashrc
gogrc --init fish | source     # ~/.config/fish/config.fish
```

Existing aliases such as `alias ls='grc --colour=auto ls'` work unchanged
with `grc` replaced by `gogrc`.

### Stock configuration

gogrc embeds the stock configuration from grc 1.14 (the `newmaster`
branch). If grc is not installed, or to use the newer files, write them out:

```sh
gogrc --write-defaults            # writes ~/.config/gogrc/defaults
gogrc --write-defaults --force    # also replaces files written earlier
```

Only `~/.config/gogrc/defaults` is written. Your own files are never
touched, and they take precedence over the defaults (see
[Configuration](#configuration)).

### Replacing grc

1. Run `gogrc --write-defaults`, so the stock color files no longer come
   from grc's installation.
2. Change your aliases or shell setup from `grc` to `gogrc`.
3. Uninstall grc (`brew uninstall grc`, `apt remove grc`, ...).

Your files in `~/.config/grc` and `~/.grc` keep working, and override the
stock ones.

## Configuration

gogrc uses your existing grc configuration.

`grc.conf` maps command lines to color files. The files are read in this
order; a match in a later file overrides one in an earlier file, and within a
file the first match wins:

1. `/etc/grc.conf`, `/usr/local/etc/grc.conf`, `/opt/homebrew/etc/grc.conf`
2. `~/.config/gogrc/defaults/grc.conf`
3. `~/.config/grc/grc.conf`
4. `~/.grc/grc.conf`
5. `~/.config/gogrc/grc.conf`

So a personal `grc.conf` only needs the commands it adds or changes.

Each color file is taken from the first of these directories that has it:

1. `~/.config/gogrc`
2. `~/.local/share/grc`
3. `~/.config/grc`
4. `~/.grc`
5. `~/.config/gogrc/defaults`
6. `/opt/homebrew/share/grc`, `/usr/local/share/grc`, `/usr/share/grc`

A color file name may include a subdirectory (`kube/conf.pods`), which is
looked up in each of these directories, and may be an absolute path.
Color files are not merged: a `conf.ls` in `~/.config/grc` replaces the
stock one entirely.

`~/.config` and `~/.local/share` follow `$XDG_CONFIG_HOME` and
`$XDG_DATA_HOME`. See grc's README for the file formats.

### ANSI handling

When a command sees a terminal it may color its own output. A color file can
choose what gogrc does with lines that already contain escape sequences, with
a directive that Python grcat ignores as a comment:

```
#gogrc: ansi=native
```

| Mode     | Lines with escape sequences |
|----------|------------------------------|
| `native` | passed through untouched (default) |
| `strip`  | the command's sequences are removed, then gogrc colors the line |
| `merge`  | matched on the text without sequences; gogrc's colors win where rules apply, the command's colors are kept elsewhere (including `unchanged` groups) |

`--ansi MODE` overrides the color file.

## Options

```
-e, --stderr        color stderr; stdout is then left alone unless -s is given
-s, --stdout        color stdout (the default), even with -e
-c, --config NAME   use color file NAME instead of looking it up in grc.conf
    --color WHEN    on, off or auto (default); auto colors when stdout is a
                    terminal and NO_COLOR is not set
    --no-pty        give the command pipes instead of pseudo-terminals
    --ansi MODE     native, strip or merge
    --cat NAME      color stdin with color file NAME, like grcat
    --init SHELL    print shell aliases (bash, zsh or fish)
    --write-defaults [--force]
```

Running the binary under the name `grcat` (for example through a symlink)
makes it behave like `gogrc --cat`.

## Differences from grc

- **The command sees a terminal.** When gogrc colors a stream and that
  stream is a terminal, the command writes to a pseudo-terminal instead of a
  pipe, so it keeps terminal behavior: columns, progress output and its own
  colors. Stdin stays the real terminal, and the command stays in your
  terminal's foreground process group, so typing, Ctrl-C and Ctrl-Z work
  normally. Window size changes are passed on. Use `--no-pty` for grc's pipe
  behavior.
- **Pagers are disabled** when the command gets a pseudo-terminal (`PAGER`,
  `GIT_PAGER`, `SYSTEMD_PAGER`, ... are set to `cat`), because a pager's
  screen output can't be colored line by line.
- **Prompts appear.** A partial line (such as `Password: `) is shown after
  50ms instead of waiting for its newline. This only happens when writing to
  a terminal.
- **No color file, no overhead.** When nothing would be colored, gogrc
  replaces itself with the command (`exec`).
- **Exit status.** A command killed by a signal makes gogrc exit the same way
  (grc exits 0). Ctrl-C reaches the command once, not twice.
- **Commands that already color their output.** See [ANSI handling](#ansi-handling).
- **Regular expressions.** gogrc uses Go's RE2 engine where it can and falls
  back to [regexp2](https://github.com/dlclark/regexp2) for lookaround and for
  lines containing non-ASCII text, where `\w`, `\d` and `\b` are
  Unicode-aware as in Python.
- **Robustness.** Bad keywords, colors and regexps in color files are
  reported and skipped instead of crashing. Rules that make grcat loop
  forever (empty matches with `count=once`, entries with only a regexp) are
  handled. A trailing `\r` is not part of the line when matching, so
  patterns ending in `$` work on CRLF output.
- **Colors.** The xterm 256-color names from grc 1.14 (`xterm_208`,
  `on_xterm_17`) are supported. `--color` and `color=` are the primary
  spellings; `--colour`, `colour=` and `colours=` still work.
- **The current directory is not searched for color files named in
  `grc.conf`.** grcat looks there first, so a `conf.ls` in whatever directory
  you are in would change `ls`, and could run commands through `command=`.
  Names given with `-c` or `--cat` that contain a `/` are still tried
  relative to the current directory first.

## Development

```sh
make test     # vet, plus unit, end-to-end (pty) and Python grcat comparison tests
make cover    # coverage, including the gogrc binaries the end-to-end tests run
make lint     # go vet, gopls and golangci-lint (configured in .golangci.yml)
make dist     # release archives in dist/, built with goreleaser
GOGRC_DIFF_CONFS=~/.config/grc go test ./internal/colorize   # compare your own color files too
```

The comparison test needs `python3`; it runs `testdata/grcat.py` (grcat from
grc's `newmaster` branch). Coverage cannot see `runner.Exec` or the part of
`runner.Exit` that kills the process: neither returns, so their coverage data
is never written, although the end-to-end tests exercise both.

| Package | Role |
|---|---|
| `internal/rx` | Python-compatible regexps: RE2 with a regexp2 fallback |
| `internal/colorfile` | color file parser, color names, `#gogrc:` directives |
| `internal/colorize` | the grcat matching and rendering engine, ANSI modes |
| `internal/stream` | line splitting, flushing and partial-line output |
| `internal/runner` | starting the command, ptys, signals, exit status |
| `internal/config` | grc.conf and color file search paths |
| `internal/defaults` | the embedded stock configuration |
| `internal/shellinit` | `--init` scripts |

## Releases

GitHub Actions runs the tests and linters on Linux and macOS for every push
and pull request. Pushing a `v*` tag builds the binaries with
[GoReleaser](https://goreleaser.com) and publishes them as a GitHub release:

```sh
git tag v0.1.0
git push origin v0.1.0
```

## License

gogrc is free software, licensed under the GNU General Public License,
version 2 or (at your option) any later version, like grc, from which its
logic and stock configuration are derived. See [LICENSE](LICENSE) and
[NOTICE](NOTICE), which credits grc and lists the files copied from it.
