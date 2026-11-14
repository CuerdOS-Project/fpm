package fpm

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// Tab completion. `fpm completion bash|zsh|fish` prints a script; the scripts
// call the hidden `fpm __complete -- <words...>` where the last word is the
// one being completed. Output is one candidate per line; a line ":files"
// asks the shell for ordinary file completion instead.

var completionTopCommands = []string{
	"install", "remove", "superremove", "search", "info", "update", "upgrade",
	"repos", "list", "repair", "clean", "check", "files", "owns", "deps", "hold", "unhold",
	"orphans", "autoremove", "size", "diskusage", "cache-info", "db", "kernel",
	"aimg", "flat", "pluto", "completion",
}

var completionGlobalFlags = []string{"-y", "--yes", "-v", "--verbose", "-V", "--version", "-h", "--help"}

var completionSubcommands = map[string][]string{
	"db":         {"list", "sync", "check", "repair"},
	"kernel":     {"list", "current", "rm"},
	"repos":      {"list", "enable", "fallback"},
	"completion": {"bash", "zsh", "fish"},
	"aimg":       {"install", "remove", "update", "list", "search", "repo", "info", "manage"},
	"flat": {"install", "remove", "update", "search", "list", "info", "remotes", "remote-add",
		"remote-remove", "remote-ls", "remote-info", "setup-flathub"},
	"pluto": {"v1", "v2"},
}

var completionCommandFlags = map[string][]string{
	"list":   {"-a", "--all", "-s", "--select"},
	"deps":   {"-r", "--reverse"},
	"search": {"--source"},
	"aimg":   {"--name", "--desc", "--category", "--icon", "--repo", "--backend", "--plain", "--interactive"},
	"flat":   {"--user", "--remote"},
}

// completionAliases maps command aliases to the key used above.
func completionCanonical(command string) string {
	switch command {
	case "kernels":
		return "kernel"
	case "appimage", "am":
		return "aimg"
	case "flatpak":
		return "flat"
	}
	return command
}

// completionQuiet runs a command capturing stdout only; stderr is dropped so
// tab presses never print diagnostics into the user's prompt.
func completionQuiet(args ...string) string {
	if len(args) == 0 || !cmdExists(args[0]) {
		return ""
	}
	var out bytes.Buffer
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return ""
	}
	return out.String()
}

func completionInstalledPackages() []string {
	var names []string
	for _, line := range strings.Split(completionQuiet("xbps-query", "-l"), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !isInstalledListStatus(fields[0]) {
			continue
		}
		name, _ := splitPlutoPackageVersion(fields[1])
		names = append(names, name)
	}
	return names
}

// completionRepoPackages searches the repositories; it needs a prefix of two
// characters so a bare Tab never triggers a huge query.
func completionRepoPackages(prefix string) []string {
	if len(prefix) < 2 || !plutoPackageName.MatchString(prefix) {
		return nil
	}
	rows := parseXBPSSearchRows(completionQuiet("xbps-query", "-Rs", prefix), "", false, 300)
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.Name)
	}
	return names
}

func completionFilter(candidates []string, prefix string) []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range candidates {
		if c == "" || seen[c] || !strings.HasPrefix(c, prefix) {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

func completionFlatpakRemotes() []string {
	return strings.Fields(completionQuiet("flatpak", "remotes", "--columns=name"))
}

// completeWords returns the candidates for words (last = word being typed).
func completeWords(words []string) []string {
	if len(words) == 0 {
		words = []string{""}
	}
	cur, prev := words[len(words)-1], words[:len(words)-1]

	// Value of an option immediately before the cursor.
	if len(prev) > 0 {
		switch prev[len(prev)-1] {
		case "--repo":
			return completionFilter([]string{amFusionRepoName}, cur)
		case "--backend":
			return completionFilter([]string{"am", "appman"}, cur)
		case "--source":
			return completionFilter([]string{"xbps", "flatpak", "am", "appimage"}, cur)
		case "--icon":
			return []string{":files"}
		case "--name", "--desc", "--category":
			return nil
		case "--remote":
			return completionFilter(completionFlatpakRemotes(), cur)
		}
	}

	// Locate the command: the first word that is not an option.
	command, pos := "", -1
	for i, w := range prev {
		if !strings.HasPrefix(w, "-") {
			command, pos = completionCanonical(w), i
			break
		}
	}
	if command == "" {
		if strings.HasPrefix(cur, "-") {
			return completionFilter(completionGlobalFlags, cur)
		}
		return completionFilter(completionTopCommands, cur)
	}
	var rest []string // non-option words between the command and the cursor
	for _, w := range prev[pos+1:] {
		if !strings.HasPrefix(w, "-") {
			rest = append(rest, w)
		}
	}
	if strings.HasPrefix(cur, "-") {
		flags := append(append([]string{}, completionCommandFlags[command]...), completionGlobalFlags...)
		return completionFilter(flags, cur)
	}

	switch command {
	case "install":
		return completionFilter(completionRepoPackages(cur), cur)
	case "info":
		if len(rest) == 0 {
			return completionFilter(append(completionInstalledPackages(), completionRepoPackages(cur)...), cur)
		}
	case "remove", "superremove", "hold", "unhold", "size":
		return completionFilter(completionInstalledPackages(), cur)
	case "files":
		if len(rest) == 0 {
			return completionFilter(completionInstalledPackages(), cur)
		}
	case "deps":
		if len(rest) == 0 {
			return completionFilter(append(completionInstalledPackages(), completionRepoPackages(cur)...), cur)
		}
	case "owns":
		return []string{":files"}
	case "search":
		return nil
	case "db", "completion":
		if len(rest) == 0 {
			return completionFilter(completionSubcommands[command], cur)
		}
	case "repos":
		switch {
		case len(rest) == 0:
			return completionFilter(completionSubcommands["repos"], cur)
		case rest[0] == "enable" && len(rest) == 1:
			names := make([]string, 0, len(voidRepoPackages))
			for name := range voidRepoPackages {
				names = append(names, name)
			}
			return completionFilter(names, cur)
		case rest[0] == "fallback" && len(rest) == 1:
			return completionFilter([]string{"status", "enable", "disable"}, cur)
		}
	case "kernel":
		switch {
		case len(rest) == 0:
			return completionFilter(completionSubcommands["kernel"], cur)
		case isKernelRemoveWord(rest[0]) && !containsString(rest[1:], "all"):
			var versions []string
			entries, _ := listKernelEntries()
			for _, entry := range entries {
				if entry.removable() {
					versions = append(versions, entry.Version)
				}
			}
			versions = append(versions, "all")
			// Offer only versions not already typed.
			var left []string
			for _, v := range versions {
				if !containsString(rest[1:], v) {
					left = append(left, v)
				}
			}
			return completionFilter(left, cur)
		}
	case "pluto":
		switch {
		case len(rest) == 0:
			return completionFilter(completionSubcommands["pluto"], cur)
		case rest[0] == "v1" && len(rest) == 1:
			return completionFilter([]string{"hello", "search"}, cur)
		case rest[0] == "v2" && len(rest) == 1:
			return completionFilter([]string{"hello", "search", "installed", "updates", "info", "mirrors", "kernels"}, cur)
		}
	case "aimg":
		if len(rest) == 0 {
			return completionFilter(completionSubcommands["aimg"], cur)
		}
		switch rest[0] {
		case "install":
			return []string{":files"}
		case "remove", "update", "info":
			var names []string
			for _, app := range scanAllAppImages() {
				names = append(names, app.Slug)
			}
			return completionFilter(names, cur)
		case "repo":
			if len(rest) == 1 {
				return completionFilter([]string{"list", "add", "install", "enable", "disable", "sync"}, cur)
			}
			if len(rest) == 2 {
				return completionFilter([]string{amFusionRepoName}, cur)
			}
		}
	case "flat":
		if len(rest) == 0 {
			return completionFilter(completionSubcommands["flat"], cur)
		}
		switch rest[0] {
		case "remove", "update", "info":
			return completionFilter(strings.Fields(completionQuiet("flatpak", "list", "--columns=application")), cur)
		case "remote-remove", "remote-ls", "remote-info":
			if len(rest) == 1 {
				return completionFilter(completionFlatpakRemotes(), cur)
			}
		}
	}
	return nil
}

func runCompleteCommand(args []string) int {
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	for _, line := range completeWords(args) {
		fmt.Println(line)
	}
	return 0
}

func runCompletionCommand(args []string) {
	shell := ""
	if len(args) > 0 {
		shell = strings.ToLower(args[0])
	} else if base := strings.TrimSpace(os.Getenv("SHELL")); base != "" {
		shell = base[strings.LastIndexByte(base, '/')+1:]
	}
	switch shell {
	case "bash":
		_, _ = os.Stdout.WriteString(bashCompletionScript)
	case "zsh":
		_, _ = os.Stdout.WriteString(zshCompletionScript)
	case "fish":
		_, _ = os.Stdout.WriteString(fishCompletionScript)
	default:
		dErr("%s", tr("err_completion_shell"))
	}
}

const bashCompletionScript = `# bash completion for fpm  (fpm completion bash)
_fpm_complete() {
    local cur="${COMP_WORDS[COMP_CWORD]}" line
    local -a words=("${COMP_WORDS[@]:1:COMP_CWORD}")
    COMPREPLY=()
    local out
    out="$(fpm __complete -- "${words[@]}" 2>/dev/null)"
    if [[ "$out" == ":files" ]]; then
        compopt -o default 2>/dev/null
        return 0
    fi
    while IFS= read -r line; do
        [[ -n "$line" ]] && COMPREPLY+=("$line")
    done <<< "$out"
}
complete -F _fpm_complete fpm
`

const zshCompletionScript = `#compdef fpm
# zsh completion for fpm  (fpm completion zsh > ~/.zfunc/_fpm)
_fpm() {
    local -a candidates
    local out
    out="${(@f)$(fpm __complete -- "${(@)words[2,CURRENT]}" 2>/dev/null)}"
    if [[ "$out" == ":files" ]]; then
        _files
        return
    fi
    candidates=("${(@f)out}")
    candidates=(${candidates:#})
    (( ${#candidates} )) && compadd -a candidates
}
compdef _fpm fpm
`

const fishCompletionScript = `# fish completion for fpm  (fpm completion fish > ~/.config/fish/completions/fpm.fish)
function __fpm_complete
    set -l words (commandline -opc)[2..-1] (commandline -ct)
    set -l out (fpm __complete -- $words 2>/dev/null)
    if test "$out" = ":files"
        __fish_complete_path (commandline -ct)
        return
    end
    printf '%s\n' $out
end
complete -c fpm -f -a '(__fpm_complete)'
`
