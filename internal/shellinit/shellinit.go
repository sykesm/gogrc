// SPDX-License-Identifier: GPL-2.0-or-later

// Package shellinit generates shell code that wraps common commands with
// gogrc.
package shellinit

import (
	"fmt"
	"strings"
)

// Commands is the list of commands wrapped by default, from grc's grc.zsh.
var Commands = []string{
	"as", "ant", "blkid", "cc", "configure", "curl", "cvs", "df", "diff", "dig",
	"dnf", "docker", "docker-compose", "docker-machine", "du", "env", "fdisk",
	"findmnt", "free", "g++", "gas", "gcc", "getfacl", "getsebool", "gmake", "id",
	"ifconfig", "iostat", "ip", "iptables", "iwconfig", "journalctl", "kubectl",
	"last", "ldap", "ld", "ls", "lsattr", "lsblk", "lsmod", "lsof", "lspci",
	"make", "mount", "mtr", "mvn", "netstat", "nmap", "ntpdate", "php", "ping",
	"ping6", "podman", "proftpd", "ps", "sar", "semanage", "sensors", "showmount",
	"sockstat", "ss", "stat", "sysctl", "systemctl", "tcpdump", "traceroute",
	"traceroute6", "tune2fs", "ulimit", "uptime", "vmstat", "wdiff", "whois",
}

// Script returns the init script for shell (bash, zsh or fish).
func Script(shell string) (string, error) {
	list := strings.Join(Commands, " ")
	switch shell {
	case "bash", "sh":
		return fmt.Sprintf(`# gogrc aliases. Add to ~/.bashrc:  eval "$(gogrc --init bash)"
if [ -t 1 ] && [ -n "$TERM" ] && [ "$TERM" != dumb ] && command -v gogrc >/dev/null 2>&1; then
  for cmd in %s; do
    if command -v "$cmd" >/dev/null 2>&1; then
      alias "$cmd=gogrc $cmd"
    fi
  done
  unset cmd
fi
`, list), nil
	case "zsh":
		return fmt.Sprintf(`# gogrc aliases. Add to ~/.zshrc:  eval "$(gogrc --init zsh)"
if [[ -t 1 && -n "$TERM" && "$TERM" != dumb ]] && (( $+commands[gogrc] )); then
  for cmd in %s; do
    (( $+commands[$cmd] )) && alias "$cmd=gogrc $cmd"
  done
  unset cmd
fi
`, list), nil
	case "fish":
		return fmt.Sprintf(`# gogrc wrappers. Add to ~/.config/fish/config.fish:  gogrc --init fish | source
if isatty stdout; and type -q gogrc
  for cmd in %s
    if type -q $cmd
      function $cmd --inherit-variable cmd --wraps=$cmd
        command gogrc $cmd $argv
      end
    end
  end
end
`, list), nil
	}
	return "", fmt.Errorf("unsupported shell %q (want bash, zsh or fish)", shell)
}
