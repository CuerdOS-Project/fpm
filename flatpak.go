package main

import (
	"fmt"
	"strings"
)

type FlatpakOptions struct {
	Remote string
	User   bool
}

type FlatpakRemote struct {
	Name    string
	Title   string
	URL     string
	Options string
}

func flatpakAvailable() bool { return cmdExists("flatpak") }
func checkFlatpak() bool {
	if !flatpakAvailable() {
		dErr("%s", tr("fp_err_not_found"))
		return false
	}
	return true
}

func flatpakCommand(options FlatpakOptions, subcommand string, args ...string) []string {
	command := []string{"flatpak"}
	if options.User {
		command = append(command, "--user")
	}
	command = append(command, subcommand)
	if yesFlag && (subcommand == "install" || subcommand == "uninstall" || subcommand == "update") {
		command = append(command, "-y")
	}

	if options.Remote != "" && subcommand == "install" {
		command = append(command, options.Remote)
	}
	return append(command, args...)
}

func installedFlatpaks(packages []string, options FlatpakOptions) []string {
	result := runCmd(flatpakCommand(options, "list", "--columns=application"), true)
	installed := make([]string, 0, len(packages))
	for _, pkg := range packages {
		if result.Stdout != "" && packageInInstalledList(pkg, result.Stdout) {
			installed = append(installed, pkg)
			continue
		}
		if result.Stdout == "" && runCmd(flatpakCommand(options, "info", pkg), true).Code == 0 {
			installed = append(installed, pkg)
		}
	}
	return installed
}

func installFlatpak(packages []string, options FlatpakOptions) {
	if len(packages) == 0 || !checkFlatpak() {
		return
	}
	installed := installedFlatpaks(packages, options)
	pending := packages
	if len(installed) > 0 {
		fmt.Printf("\n%s%s:%s %s\n", bold, tr("lbl_already_installed"), reset, joinArgs(installed))
		action := askExistingAction()
		if action != 1 {
			if action == 2 {
				removeFlatpak(installed, options)
			}
			pending = make([]string, 0, len(packages))
			for _, pkg := range packages {
				if !containsString(installed, pkg) {
					pending = append(pending, pkg)
				}
			}
			if len(pending) == 0 {
				if action == 2 {
					dInfo("%s", tr("info_all_installed_removed"))
				} else {
					dInfo("%s", tr("info_all_installed_skipped"))
				}
				return
			}
		}
	}
	header := tr("fp_hdr_installing")
	if options.Remote != "" {
		header = fmt.Sprintf("%s [%s]", header, options.Remote)
	}
	dHeader("%s: %s", header, joinArgs(pending))
	r := runCmd(flatpakCommand(options, "install", pending...), false)
	if r.Code == 0 {
		dOK("%s", tr("fp_ok_installed"))
		notifyYelenaUpdateManager()
	} else {
		dErr("%s", tr("fp_err_install_failed"))
	}
}

func removeFlatpak(packages []string, options FlatpakOptions) {
	if len(packages) == 0 || !checkFlatpak() {
		return
	}
	dHeader("%s: %s", tr("fp_hdr_removing"), joinArgs(packages))
	r := runCmd(flatpakCommand(options, "uninstall", packages...), false)
	if r.Code == 0 {
		dOK("%s", tr("fp_ok_removed"))
		notifyYelenaUpdateManager()
	} else {
		dErr("%s", tr("fp_err_remove_failed"))
	}
}

func updateFlatpak(packages []string, options FlatpakOptions) {
	if !checkFlatpak() {
		return
	}
	if len(packages) == 0 {
		dHeader("%s", tr("fp_hdr_updating_all"))
	} else {
		dHeader("%s", tr("fp_hdr_updating"))
	}
	r := runCmd(flatpakCommand(options, "update", packages...), false)
	if r.Code == 0 {
		dOK("%s", tr("fp_ok_updated"))
		notifyYelenaUpdateManager()
	} else {
		dErr("%s", tr("fp_err_update_failed"))
	}
}

func parseRemoteColumns(output string) []FlatpakRemote {
	remotes := make([]FlatpakRemote, 0)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		remote := FlatpakRemote{}
		if len(fields) > 0 {
			remote.Name = fields[0]
		}
		if len(fields) > 1 {
			remote.Title = fields[1]
		}
		if len(fields) > 2 {
			remote.URL = fields[2]
		}
		if len(fields) > 3 {
			remote.Options = fields[3]
		}
		remotes = append(remotes, remote)
	}
	return remotes
}

func listFlatpakRemotes(options FlatpakOptions) {
	if !checkFlatpak() {
		return
	}
	dHeader("%s", tr("fp_hdr_remotes"))
	r := runCmd(flatpakCommand(options, "remotes", "--columns=name,title,url,options"), true)
	for _, remote := range parseRemoteColumns(r.Stdout) {
		fmt.Printf("  %s%-16s%s %-28s %s %s\n", cyan, remote.Name, reset, remote.Title, remote.URL, remote.Options)
	}
	if strings.TrimSpace(r.Stdout) == "" {
		dInfo("%s", tr("fp_info_no_remotes"))
	}
}

func addFlatpakRemote(name, location string, options FlatpakOptions) {
	if !checkFlatpak() || name == "" || location == "" {
		dErr("%s", tr("fp_err_remote_args"))
		return
	}
	if !askConfirm(tr("fp_ask_add_remote")) {
		dInfo("%s", tr("info_install_cancelled"))
		return
	}
	args := []string{"remote-add", "--if-not-exists", name, location}
	r := runCmd(flatpakCommand(options, "remote-add", args[1:]...), false)
	if r.Code == 0 {
		dOK("%s", tr("fp_ok_remote_added"))
	} else {
		dErr("%s", tr("fp_err_remote_add_failed"))
	}
}
func removeFlatpakRemote(name string, options FlatpakOptions) {
	if !checkFlatpak() || name == "" {
		dErr("%s", tr("fp_err_remote_args"))
		return
	}
	if !askConfirm(tr("fp_ask_remove_remote")) {
		dInfo("%s", tr("info_remove_cancelled"))
		return
	}
	r := runCmd(flatpakCommand(options, "remote-delete", name), false)
	if r.Code == 0 {
		dOK("%s", tr("fp_ok_remote_removed"))
	} else {
		dErr("%s", tr("fp_err_remote_remove_failed"))
	}
}
func infoFlatpakRemote(remote, ref string, options FlatpakOptions) {
	if !checkFlatpak() {
		return
	}
	r := runCmd(flatpakCommand(options, "remote-info", remote, ref), true)
	if r.Code != 0 || r.Stdout == "" {
		dErr(tr("fp_err_remote_info"), remote, ref)
		return
	}
	dHeader("%s: %s/%s", tr("fp_hdr_remote_info"), remote, ref)
	fmt.Println(r.Stdout)
}
func listFlatpakRemote(remote string, options FlatpakOptions) {
	if !checkFlatpak() {
		return
	}
	args := []string{"--app", "--columns=name,application,version,description,remotes"}
	if remote != "" {
		args = append(args, remote)
	}
	r := runCmd(flatpakCommand(options, "remote-ls", args...), true)
	if r.Code != 0 || strings.TrimSpace(r.Stdout) == "" {
		dInfo("%s", tr("fp_info_no_remote_apps"))
		return
	}
	dHeader("%s", tr("fp_hdr_remote_list"))
	fmt.Print(r.Stdout)
}
func setupFlathub(options FlatpakOptions) {
	addFlatpakRemote("flathub", "https://dl.flathub.org/repo/flathub.flatpakrepo", options)
}

func searchFlatpak(query string, options FlatpakOptions) {
	if !checkFlatpak() {
		return
	}
	dHeader("%s: '%s'", tr("fp_hdr_searching"), query)
	r := runCmd(flatpakCommand(options, "search", "--columns=name,description,application,remotes", query), true)
	if r.Stdout == "" {
		dWarn("%s", tr("fp_warn_no_packages_found"))
		return
	}
	installed := runCmd(flatpakCommand(options, "list", "--columns=application"), true)
	items := parseSearchLines(r.Stdout, "Flatpak", installed.Stdout)
	if !isTTY() {
		for _, item := range items {
			status := ""
			if item.Installed {
				status = " [" + tr("tui_installed") + "]"
			}
			remote := item.Remote
			if remote != "" {
				remote = " [" + remote + "]"
			}
			fmt.Printf("  %s*%s %s%s  %s%s\n", green, reset, item.Label, remote, item.Desc, status)
		}
		return
	}
	selected, _ := selectItems("FPM Flatpak", items, false, nil)
	if len(selected) == 0 {
		dInfo("%s", tr("info_none_selected"))
		return
	}
	groups := make(map[string][]string)
	for _, item := range selected {
		groups[item.Remote] = append(groups[item.Remote], item.Name)
	}
	if !askConfirm(tr("fp_ask_install_selected")) {
		dInfo("%s", tr("info_install_cancelled"))
		return
	}
	for remote, packages := range groups {
		installFlatpak(packages, FlatpakOptions{Remote: remote, User: options.User})
	}
}

func listFlatpak(options FlatpakOptions) {
	if !checkFlatpak() {
		return
	}
	dHeader("%s", tr("fp_hdr_list"))
	r := runCmd(flatpakCommand(options, "list", "--columns=name,application,version,origin"), true)
	if strings.TrimSpace(r.Stdout) == "" {
		dInfo("%s", tr("fp_info_none_installed"))
		return
	}
	for _, line := range strings.Split(strings.TrimSpace(r.Stdout), "\n") {
		if line != "" {
			fmt.Printf("  %s*%s %s\n", green, reset, line)
		}
	}
}
func infoFlatpak(pkg string, options FlatpakOptions) {
	if !checkFlatpak() {
		return
	}
	r := runCmd(flatpakCommand(options, "info", pkg), true)
	if r.Code != 0 || r.Stdout == "" {
		dErr(tr("fp_err_package_not_found"), pkg)
		return
	}
	dHeader("%s: %s", tr("fp_hdr_info"), pkg)
	fmt.Println(r.Stdout)
}
