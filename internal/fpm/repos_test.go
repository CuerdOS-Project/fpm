package fpm

import "testing"

func TestVoidRepoPackages(t *testing.T) {
	cases := map[string]string{
		"nonfree":          "void-repo-nonfree",
		"multilib":         "void-repo-multilib",
		"multilib-nonfree": "void-repo-multilib-nonfree",
		"multilib/nonfree": "void-repo-multilib-nonfree",
		"debug":            "void-repo-debug",
	}
	for name, want := range cases {
		if got := voidRepoPackages[name]; got != want {
			t.Errorf("voidRepoPackages[%q] = %q, want %q", name, got, want)
		}
	}
}

func TestReposCommandRootRequirement(t *testing.T) {
	if reposCommandNeedsRoot([]string{"list"}) || reposCommandNeedsRoot(nil) ||
		reposCommandNeedsRoot([]string{"enable", "current"}) ||
		reposCommandNeedsRoot([]string{"enable", "musl"}) ||
		reposCommandNeedsRoot([]string{"fallback", "status"}) ||
		reposCommandNeedsRoot([]string{"enable", "unknown"}) {
		t.Fatal("repository listing should not require root")
	}
	if !reposCommandNeedsRoot([]string{"enable", "nonfree"}) {
		t.Fatal("enabling a repository must use the privileged XBPS install flow")
	}
	if !reposCommandNeedsRoot([]string{"fallback", "enable"}) ||
		!reposCommandNeedsRoot([]string{"fallback", "disable"}) {
		t.Fatal("changing the system-wide fallback configuration must be privileged")
	}
}
