package fpm

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

func checkXBPS() bool {
	if !cmdExists("xbps-install") {
		dErr("%s", tr("err_xbps_not_found"))
		return false
	}
	return true
}

func askConfirm(question string) bool {
	if yesFlag {
		return true
	}
	answer := readLine(fmt.Sprintf("%s?%s %s [s/N]", yellow, reset, question), "")
	return strings.EqualFold(answer, "s") || strings.EqualFold(answer, "y")
}

func askExistingAction() int {
	if yesFlag {
		return 1
	}
	answer := readLine(fmt.Sprintf("%s?%s %s [R/O/E]", yellow, reset, tr("ask_reinstall_skip_or_remove")), "")
	switch strings.ToLower(answer) {
	case "r", "y":
		return 1
	case "e":
		return 2
	default:
		return 0
	}
}

// packageInInstalledList is the one-shot lookup. Code that checks many
// packages against the same listing must build newInstalledLineIndex once.
func packageInInstalledList(pkg, output string) bool {
	return newInstalledLineIndex(output).has(pkg)
}

func installedXBPS(packages []string) []string {
	result := runCmd([]string{"xbps-query", "-l"}, true)
	installed := make([]string, 0, len(packages))
	if result.Stdout != "" {
		index := newInstalledLineIndex(result.Stdout)
		for _, pkg := range packages {
			if index.has(pkg) {
				installed = append(installed, pkg)
			}
		}
		return installed
	}
	for _, pkg := range packages {
		r := runCmd([]string{"xbps-query", "-l", pkg}, true)
		if r.Code == 0 && r.Stdout != "" {
			installed = append(installed, pkg)
		}
	}
	return installed
}

func containsString(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}

func installXBPS(packages []string) {
	if len(packages) == 0 || !checkXBPS() {
		return
	}
	installed := installedXBPS(packages)
	pending := packages
	if len(installed) > 0 {
		fmt.Printf("\n%s%s:%s %s\n", bold, tr("lbl_already_installed"), reset, joinArgs(installed))
		action := askExistingAction()
		if action != 1 {
			if action == 2 {
				removeXBPS(installed)
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
	dHeader("%s: %s", tr("hdr_installing"), joinArgs(pending))
	r := runCmd(xbpsCommand("xbps-install", "-S", pending...), false)
	if r.Code == 0 {
		dOK("%s", tr("ok_installed"))
		syncFPMDatabaseAfterChange()
		notifyYelenaUpdateManager()
	} else {
		dErr("%s", tr("err_install_failed"))
	}
}

func removeXBPS(packages []string) {
	if len(packages) == 0 || !checkXBPS() {
		return
	}
	dHeader("%s: %s", tr("hdr_removing"), joinArgs(packages))
	r := runCmd(xbpsCommand("xbps-remove", "-R", packages...), false)
	if r.Code == 0 {
		dOK("%s", tr("ok_removed"))
		syncFPMDatabaseAfterChange()
		notifyYelenaUpdateManager()
	} else {
		dErr("%s", tr("err_remove_failed"))
	}
}

func superRemove(packages []string) {
	if len(packages) == 0 || !checkXBPS() {
		return
	}
	dHeader("Superremove: %s", joinArgs(packages))
	dStep("%s", tr("step_removing_config"))
	runCmd(xbpsCommand("xbps-remove", "-RF", packages...), false)
	dStep("%s", tr("step_removing_orphans"))
	runCmd(xbpsCommand("xbps-remove", "-o"), false)
	dStep("%s", tr("step_cleaning_cache"))
	runCmd(xbpsCommand("xbps-remove", "-O"), false)
	dOK("%s", tr("ok_superremove_done"))
	syncFPMDatabaseAfterChange()
	notifyYelenaUpdateManager()
}

func updateXBPS() {
	if !checkXBPS() {
		return
	}
	dHeader("%s", tr("hdr_syncing"))
	r := runCmd(xbpsCommand("xbps-install", "-S"), false)
	if r.Code == 0 {
		dOK("%s", tr("ok_synced"))
		notifyYelenaUpdateManager()
		listPendingXBPSUpdates()
	} else {
		dErr("%s", tr("err_sync_failed"))
	}
}

func upgradeXBPS() {
	if !checkXBPS() {
		return
	}
	dHeader("%s", tr("hdr_upgrading"))
	cleanupUpdateActivity := beginYelenaUpdateActivity()
	defer cleanupUpdateActivity()
	r := runCmd(xbpsCommand("xbps-install", "-Su"), false)
	if r.Code == 0 {
		dOK("%s", tr("ok_upgraded"))
		syncFPMDatabaseAfterChange()
		notifyYelenaUpdateManager()
	} else {
		dErr("%s", tr("err_upgrade_failed"))
	}
}

func parseSearchLines(output string, source string, installed string) []TuiItem {
	var items []TuiItem
	installedIndex := newInstalledLineIndex(installed)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		name, label, desc, remote := line, line, "", ""
		if source == "XBPS" {
			if bracket := strings.IndexByte(line, ']'); strings.HasPrefix(line, "[") && bracket >= 0 {
				line = strings.TrimSpace(line[bracket+1:])
			}
			fields := strings.SplitN(line, " ", 2)
			name = fields[0]
			label = name
			if len(fields) == 2 {
				desc = strings.TrimSpace(fields[1])
			}
		} else {
			fields := strings.Split(line, "\t")
			label = fields[0]
			name = fields[0]
			if len(fields) > 1 {
				desc = fields[1]
			}
			if len(fields) > 2 {
				name = fields[2]
			}
			if len(fields) > 3 {
				remote = fields[3]
			}
		}
		if name == "" {
			continue
		}
		items = append(items, TuiItem{Name: name, Label: label, Desc: desc, Source: source, Remote: remote,
			Installed: installedIndex.has(name), Reinstallable: true, Selectable: true})

	}
	return items
}

// searchXBPSItems, searchFlatpakItems y searchAppImageItems consultan una sola
// fuente cada una; searchAll las ejecuta solo si el filtro las admite.
func searchXBPSItems(text string) []TuiItem {
	xbps := runCmd([]string{"xbps-query", "-Rs", text}, true)
	installed := runCmd([]string{"xbps-query", "-l"}, true)
	return dedupeXBPSItems(parseSearchLines(xbps.Stdout, "XBPS", installed.Stdout))
}

func searchFlatpakItems(text string) []TuiItem {
	if !cmdExists("flatpak") {
		return nil
	}
	// Con la columna de remotos, cada resultado se instala desde su origen.
	fp := runCmd(flatpakCommand(FlatpakOptions{}, "search", "--columns=name,description,application,remotes", text), true)
	installed := runCmd(flatpakCommand(FlatpakOptions{}, "list", "--columns=application"), true)
	return parseSearchLines(fp.Stdout, "Flatpak", installed.Stdout)
}

func searchAppImageItems(text string) []TuiItem {
	apps := scanAllAppImages()
	items := make([]TuiItem, 0, len(apps))
	for _, app := range apps {
		if containsFold(app.Name, text) || containsFold(app.Slug, text) {
			items = append(items, TuiItem{Name: app.Name, Label: app.Name, Desc: app.AppImagePath, Source: "AppImage", Installed: true, Reinstallable: false, Selectable: false})
		}
	}
	return items
}

// searchAll busca en todas las fuentes (XBPS, Flatpak, am-FUSION y AppImage
// locales). La consulta admite `@fuente` para limitarla (ver itemFilter): las
// fuentes excluidas ni siquiera se consultan. XBPS se resuelve primero para
// abrir la TUI al instante; el resto se consulta en paralelo y se añade a la
// lista conforme termina.
func searchAll(rawQuery string) {
	filter := compileItemFilter(rawQuery)
	query := filter.text()
	if query == "" {
		dErr("%s", tr("err_missing_query"))
		return
	}
	if filter.mask.has(sourceXBPS) && !checkXBPS() {
		return
	}
	dHeader("%s: '%s'", tr("hdr_searching"), query)
	tty := isTTY()
	var items []TuiItem
	if filter.mask.has(sourceXBPS) {
		items = searchXBPSItems(query)
	}

	updates := make(chan []TuiItem, int(sourceKindCount)) // un envío por fuente como máximo
	var wg sync.WaitGroup
	var amErr error // solo la escribe la goroutine de AM; se lee tras cerrar `updates`
	launch := func(kind sourceKind, run func() []TuiItem) {
		if !filter.mask.has(kind) {
			return
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if batch := run(); len(batch) > 0 {
				updates <- batch
			}
		}()
	}
	launch(sourceFlatpak, func() []TuiItem { return searchFlatpakItems(query) })
	launch(sourceAM, func() []TuiItem {
		batch, err := searchAMFusionItems(query)
		if err != nil {
			amErr = err
			if tty {
				batch = append(batch, TuiItem{Label: "am-FUSION", Desc: err.Error(), Source: "AM", Selectable: false})
			}
		}
		return batch
	})
	launch(sourceAppImage, func() []TuiItem { return searchAppImageItems(query) })
	go func() {
		wg.Wait()
		close(updates)
	}()

	if tty {
		resume := &TuiResume{}
		if filter.mask != 0 {
			resume.Query = withSourceMask("", filter.mask) // abre la TUI con la fuente ya marcada
		}
		for {
			selected, activated := selectItems("FPM - Search", items, true, resume, updates)
			if activated != nil && activated.Source == "AppImage" {
				manageAppByName(activated.Name)
				continue
			}
			if len(selected) == 0 {
				dInfo("%s", tr("info_none_selected"))
				return
			}
			var xbps []string
			flatpakGroups := make(map[string][]string)
			var amFusion []string
			for _, item := range selected {
				if item.Source == "XBPS" {
					xbps = append(xbps, item.Name)
				}
				if item.Source == "Flatpak" {
					flatpakGroups[item.Remote] = append(flatpakGroups[item.Remote], item.Name)
				}
				if item.Source == amFusionRepoName {
					amFusion = append(amFusion, item.Name)
				}
			}
			if askConfirm(tr("ask_install_selected")) {
				if len(xbps) > 0 {
					installXBPS(xbps)
				}
				for remote, packages := range flatpakGroups {
					installFlatpak(packages, FlatpakOptions{Remote: remote})
				}
				if len(amFusion) > 0 {
					installAMFusionPackages(amFusion, nil)
				}
			} else {
				dInfo("%s", tr("info_install_cancelled"))
			}
			return
		}
	}

	for batch := range updates {
		items = append(items, batch...)
	}
	if amErr != nil {
		dWarn("No se pudieron consultar resultados de am-FUSION: %v", amErr)
	}
	// Las fuentes terminan en cualquier orden; la salida se agrupa por fuente.
	for i := range items {
		items[i].kind = kindOfSource(items[i].Source)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].kind < items[j].kind })
	for _, item := range items {
		status := ""
		if item.Installed {
			status = " [" + tr("tui_installed") + "]"
		}
		fmt.Printf("  %s*%s [%-9s] %-28s  %s%s\n", green, reset, item.Source, item.Label, item.Desc, status)
	}
	if len(items) == 0 {
		dWarn("%s", tr("warn_no_packages_found"))
	}
}

func listXBPS(showAll, selectMode bool) {
	if !checkXBPS() {
		return
	}
	r := runCmd([]string{"xbps-query", "-l"}, true)
	var items []TuiItem
	for _, line := range strings.Split(strings.TrimSpace(r.Stdout), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		name := fields[0]
		if len(fields) > 1 {
			name = fields[1]
		}
		items = append(items, TuiItem{Name: name, Label: name, Source: "XBPS", Installed: true, Reinstallable: true, Selectable: true})
	}
	if selectMode {
		if !isTTY() {
			dErr("%s", tr("err_select_needs_tty"))
			return
		}
		selected, _ := selectItems("FPM", items, false, nil)
		if len(selected) == 0 {
			dInfo("%s", tr("info_none_selected"))
			return
		}
		if askConfirm(tr("ask_remove_selected")) {
			removeXBPS(namesOf(selected))
		} else {
			dInfo("%s", tr("info_remove_cancelled"))
		}
		return
	}
	dHeader("%s (%d)", tr("hdr_installed_packages"), len(items))
	limit := len(items)
	if !showAll && limit > 50 {
		limit = 50
	}
	for _, item := range items[:limit] {
		fmt.Printf("  %s*%s %s\n", green, reset, item.Name)
	}
	if !showAll && len(items) > 50 {
		dInfo("%s", fmt.Sprintf(tr("info_more_packages"), len(items)-50))
	}
}

func infoXBPS(pkg string) {
	if !checkXBPS() {
		return
	}
	r := runCmd([]string{"xbps-query", "-R", pkg}, true)
	if r.Code != 0 || r.Stdout == "" {
		dErr(tr("err_package_not_found"), pkg)
		dInfo(tr("info_try_search"), pkg)
		return
	}
	dHeader("%s: %s", tr("hdr_info"), pkg)
	fmt.Println(r.Stdout)
}

func repairXBPS() {
	if !checkXBPS() {
		return
	}
	dHeader("%s", tr("hdr_repairing"))
	dStep("%s", tr("step_reconfiguring_db"))
	runCmd([]string{"xbps-pkgdb", "-a"}, false)
	dStep("%s", tr("step_removing_orphans"))
	runCmd(xbpsCommand("xbps-remove", "-o"), false)
	dStep("%s", tr("step_cleaning_cache"))
	runCmd(xbpsCommand("xbps-remove", "-O"), false)
	dOK("%s", tr("ok_repair_done"))
	syncFPMDatabaseAfterChange()
	notifyYelenaUpdateManager()
}

func cleanXBPS() {
	cleanXBPSCache()
}
func checkXBPSIntegrity() {
	checkXBPSDatabase()
}

func filesXBPS(pkg string) {
	if !checkXBPS() {
		return
	}
	r := runCmd([]string{"xbps-query", "-f", pkg}, true)
	if r.Code != 0 || r.Stdout == "" {
		dErr(tr("err_package_not_installed"), pkg)
		return
	}
	dHeader("%s: %s", tr("hdr_files_of"), pkg)
	fmt.Println(r.Stdout)
}
func ownsXBPS(path string) {
	if !checkXBPS() {
		return
	}
	dHeader("%s: %s", tr("hdr_searching_owner"), path)
	r := runCmd([]string{"xbps-query", "-o", path}, true)
	if r.Stdout == "" {
		dWarn("%s", tr("warn_no_owner"))
	} else {
		fmt.Println(r.Stdout)
	}
}
func depsXBPS(pkg string, reverse bool) {
	if !checkXBPS() {
		return
	}
	flag := "-x"
	label := tr("lbl_dependencies_of")
	if reverse {
		flag = "-X"
		label = tr("lbl_dependents_of")
	}
	r := runCmd([]string{"xbps-query", flag, pkg}, true)
	if r.Code != 0 {
		dErr(tr("err_package_not_found_simple"), pkg)
		return
	}
	dHeader("%s: %s", label, pkg)
	for _, line := range strings.Split(strings.TrimSpace(r.Stdout), "\n") {
		if line != "" {
			fmt.Printf("  %s*%s %s\n", green, reset, line)
		}
	}
}

func holdXBPS(packages []string, unhold bool) {
	if !checkXBPS() {
		return
	}
	mode, verb := "hold", tr("verb_holding")
	if unhold {
		mode, verb = "unhold", tr("verb_unholding")
	}
	dHeader("%s: %s", verb, joinArgs(packages))
	for _, pkg := range packages {
		runCmd([]string{"xbps-pkgdb", "-m", mode, pkg}, false)
	}
	if unhold {
		dOK("%s", tr("ok_unheld"))
	} else {
		dOK("%s", tr("ok_held"))
	}
}

func orphansXBPS(remove bool) {
	if !checkXBPS() {
		return
	}
	if remove {
		dHeader("%s", tr("hdr_autoremove"))
		r := runCmd(xbpsCommand("xbps-remove", "-o"), false)
		if r.Code == 0 {
			dOK("%s", tr("ok_autoremove_done"))
			syncFPMDatabaseAfterChange()
			notifyYelenaUpdateManager()
		} else {
			dWarn("%s", tr("warn_nothing_to_remove"))
		}
		return
	}
	dHeader("%s", tr("hdr_orphans"))
	r := runCmd(xbpsCommand("xbps-remove", "-o"), false)
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(r.Stdout), "\n") {
		if line != "" {
			fmt.Printf("  %s*%s %s\n", yellow, reset, line)
			count++
		}
	}
	if count == 0 {
		dInfo("%s", tr("info_no_orphans"))
	} else {
		dInfo(tr("info_orphans_hint"), count)
	}
}

func sizeXBPS(packages []string) {
	if !checkXBPS() {
		return
	}
	dHeader("%s", tr("hdr_size"))
	for _, pkg := range packages {
		r := runCmd([]string{"xbps-query", "-R", pkg}, true)
		if r.Code != 0 || r.Stdout == "" {
			dErr(tr("err_package_not_found_simple"), pkg)
			continue
		}
		value := tr("info_size_unknown")
		for _, line := range strings.Split(r.Stdout, "\n") {
			if strings.HasPrefix(line, "installed_size:") {
				value = strings.TrimSpace(strings.TrimPrefix(line, "installed_size:"))
				break
			}
		}
		fmt.Printf("  %s*%s %-24s %s\n", green, reset, pkg, value)
	}
}
func diskUsage() {
	dHeader("%s", tr("hdr_diskusage"))
	for _, path := range []string{"/var/cache/xbps", "/var/db/xbps"} {
		r := runCmd([]string{"du", "-sh", path}, true)
		if r.Code == 0 && r.Stdout != "" {
			fmt.Printf("  %s", r.Stdout)
		} else {
			fmt.Printf("  %s: %s\n", path, tr("info_size_unknown"))
		}
	}
}

func namesOf(items []TuiItem) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.Name)
	}
	return result
}
