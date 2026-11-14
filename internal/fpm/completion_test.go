package fpm

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestCompleteTopLevelAndFlags(t *testing.T) {
	if got := completeWords([]string{"ke"}); !reflect.DeepEqual(got, []string{"kernel"}) {
		t.Errorf("ke -> %v", got)
	}
	if got := completeWords([]string{"re"}); !reflect.DeepEqual(got, []string{"remove", "repair", "repos"}) {
		t.Errorf("re -> %v", got)
	}
	if got := completeWords([]string{"--v"}); !reflect.DeepEqual(got, []string{"--verbose", "--version"}) {
		t.Errorf("--v -> %v", got)
	}
	if got := completeWords([]string{"list", "--"}); !contains(got, "--all") || !contains(got, "--yes") {
		t.Errorf("list flags -> %v", got)
	}
}

func contains(items []string, v string) bool { return containsString(items, v) }

func TestCompleteSubcommands(t *testing.T) {
	cases := []struct {
		words []string
		want  []string
	}{
		{[]string{"db", ""}, []string{"check", "list", "repair", "sync"}},
		{[]string{"repos", "enable", "multilib-"}, []string{"multilib-nonfree"}},
		{[]string{"repos", "fallback", "e"}, []string{"enable"}},
		{[]string{"kernel", "r"}, []string{"rm"}},
		{[]string{"-y", "pluto", "v2", "k"}, []string{"kernels"}},
		{[]string{"flat", "setup"}, []string{"setup-flathub"}},
		{[]string{"aimg", "repo", "a"}, []string{"add"}},
		{[]string{"aimg", "install", "--repo", "am"}, []string{"am-FUSION"}},
		{[]string{"completion", "f"}, []string{"fish"}},
		{[]string{"kernels", "rm"}, nil},
	}
	for _, tc := range cases {
		got := completeWords(tc.words)
		if tc.want == nil {
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("completeWords(%v) = %v, want %v", tc.words, got, tc.want)
		}
	}
	if got := completeWords([]string{"owns", ""}); !reflect.DeepEqual(got, []string{":files"}) {
		t.Errorf("owns -> %v", got)
	}
}

func TestCompleteUsesXbpsAndKernelInventory(t *testing.T) {
	dir := t.TempDir()
	fakeExecutable(t, dir, "xbps-query", `case "$1" in
-l) printf 'ii vim-9.1_1 editor\nii vim-common-9.1_1 files\nii git-2.45_1 dvcs\n';;
-Rs) printf '[-] vim-9.1_1 editor\n[-] vim-9.2_1 editor\n[-] vimb-3_1 browser\n';;
esac`)
	fakeExecutable(t, dir, "uname", `echo 6.12.1_1`)
	fakeKernelTree(t, "6.1.90_1", "6.6.30_1")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if got := completeWords([]string{"remove", "vim"}); !reflect.DeepEqual(got, []string{"vim", "vim-common"}) {
		t.Errorf("remove vim -> %v", got)
	}
	if got := completeWords([]string{"install", "vi"}); !reflect.DeepEqual(got, []string{"vim", "vimb"}) {
		t.Errorf("install vi -> %v (repeated versions must collapse)", got)
	}
	if got := completeWords([]string{"install", "v"}); len(got) != 0 {
		t.Errorf("one-letter prefix must not query the repos: %v", got)
	}
	if got := completeWords([]string{"kernel", "rm", ""}); !reflect.DeepEqual(got, []string{"6.1.90_1", "6.6.30_1", "all"}) {
		t.Errorf("kernel rm -> %v", got)
	}
	if got := completeWords([]string{"kernel", "rm", "6.1.90_1", ""}); !reflect.DeepEqual(got, []string{"6.6.30_1", "all"}) {
		t.Errorf("kernel rm <v> -> %v", got)
	}
}

func TestCompletionScriptsCallHelper(t *testing.T) {
	for name, script := range map[string]string{"bash": bashCompletionScript, "zsh": zshCompletionScript, "fish": fishCompletionScript} {
		if !strings.Contains(script, "fpm __complete --") {
			t.Errorf("%s script does not call the helper", name)
		}
	}
}
