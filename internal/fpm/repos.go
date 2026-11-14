package fpm

import (
	"fmt"
	"strings"
)

var voidRepoPackages = map[string]string{
	"nonfree":          "void-repo-nonfree",
	"multilib":         "void-repo-multilib",
	"multilib-nonfree": "void-repo-multilib-nonfree",
	"multilib/nonfree": "void-repo-multilib-nonfree",
	"debug":            "void-repo-debug",
}

func reposCommandNeedsRoot(args []string) bool {
	if len(args) == 2 && strings.EqualFold(args[0], "fallback") &&
		(strings.EqualFold(args[1], "enable") || strings.EqualFold(args[1], "disable")) {
		return true
	}
	if len(args) != 2 || !strings.EqualFold(args[0], "enable") {
		return false
	}
	_, ok := voidRepoPackages[strings.ToLower(strings.TrimSpace(args[1]))]
	return ok
}

func runReposCommand(args []string) {
	if len(args) == 0 || (len(args) == 1 && (args[0] == "list" || args[0] == "status")) {
		listXBPSRepositories()
		return
	}
	if strings.EqualFold(args[0], "fallback") {
		if len(args) == 1 || (len(args) == 2 && strings.EqualFold(args[1], "status")) {
			showVoidMirrorFallbackStatus()
			return
		}
		if len(args) == 2 && strings.EqualFold(args[1], "enable") {
			enableVoidMirrorFallback()
			return
		}
		if len(args) == 2 && strings.EqualFold(args[1], "disable") {
			disableVoidMirrorFallback()
			return
		}
		dErr("%s", tr("repos_usage"))
		return
	}
	if len(args) == 2 && args[0] == "enable" {
		enableVoidRepository(args[1])
		return
	}
	dErr("%s", tr("repos_usage"))
}

func listXBPSRepositories() {
	if !cmdExists("xbps-query") {
		dErr("%s", tr("repos_query_failed"))
		return
	}
	result := runCmd([]string{"xbps-query", "-L"}, true)
	if result.Code != 0 {
		dErr("%s", tr("repos_query_failed"))
		return
	}
	output := strings.TrimSpace(result.Stdout)
	if output == "" {
		dInfo("%s", tr("repos_none"))
		return
	}
	dHeader("%s", tr("repos_active_header"))
	fmt.Println(output)
}

func enableVoidRepository(name string) {
	name = strings.ToLower(strings.TrimSpace(name))
	packageName, ok := voidRepoPackages[name]
	if !ok {
		switch name {
		case "current", "musl":
			dErr("%s", tr("repos_main_not_toggleable"))
		default:
			dErr("%s", fmt.Sprintf(tr("repos_unknown"), name))
		}
		return
	}
	if !checkXBPS() {
		return
	}
	dHeader("%s", fmt.Sprintf(tr("repos_enable_header"), name, packageName))
	result := runCmd(xbpsCommand("xbps-install", "-S", packageName), false)
	if result.Code != 0 {
		dErr("%s", tr("repos_enable_failed"))
		return
	}
	dOK("%s", fmt.Sprintf(tr("repos_enabled"), name))
	syncFPMDatabaseAfterChange()
	notifyYelenaUpdateManager()
}
