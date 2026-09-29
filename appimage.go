package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

type InstalledApp struct {
	Slug         string
	Name         string
	Category     string
	Icon         string
	DesktopPath  string
	AppImagePath string
	Size         int64
}

func appDirs() (binDir, appsDir, iconsDir, desktopDir string) {
	if os.Geteuid() == 0 {
		return "/usr/local/bin", "/opt/appimages", "/usr/local/share/icons", "/usr/local/share/applications"
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = os.Getenv("HOME")
	}
	if home == "" {
		home = "/tmp"
	}
	return filepath.Join(home, ".local", "bin"), filepath.Join(home, ".local", "share", "appimages"), filepath.Join(home, ".local", "share", "icons"), filepath.Join(home, ".local", "share", "applications")
}

func slugify(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return "app"
	}
	return b.String()
}

func guessName(path string) string {
	name := filepath.Base(path)
	if strings.EqualFold(filepath.Ext(name), ".appimage") {
		name = strings.TrimSuffix(name, filepath.Ext(name))
	}
	name = strings.NewReplacer("_", " ", "-", " ", ".", " ").Replace(name)
	return name
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return -1
	}
	return info.Size()
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

var inputReader = bufio.NewReader(os.Stdin)

func readLine(prompt, defaultValue string) string {
	if defaultValue != "" {
		fmt.Printf("%s%s?%s %s %s[%s]%s: ", bold, yellow, reset, prompt, dim, defaultValue, reset)
	} else {
		fmt.Printf("%s%s?%s %s: ", bold, yellow, reset, prompt)
	}
	line, err := inputReader.ReadString('\n')
	if err != nil && line == "" {
		return defaultValue
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultValue
	}
	return line
}

func askCategory() string {
	keys := []string{"ai_cat_utility", "ai_cat_development", "ai_cat_graphics", "ai_cat_network", "ai_cat_office", "ai_cat_multimedia", "ai_cat_system", "ai_cat_game", "ai_cat_education", "ai_cat_science"}
	values := []string{"Utility;", "Development;", "Graphics;", "Network;", "Office;", "AudioVideo;", "System;", "Game;", "Education;", "Science;"}
	fmt.Printf("%s%s?%s %s\n", bold, yellow, reset, tr("ai_ask_category"))
	for i, key := range keys {
		fmt.Printf("    %s%d)%s %s\n", cyan, i+1, reset, tr(key))
	}
	choice := readLine(">", "1")
	var n int
	fmt.Sscanf(choice, "%d", &n)
	if n < 1 || n > len(values) {
		n = 1
	}
	return values[n-1]
}

func parseDesktop(path, appsDir string) (InstalledApp, bool) {
	file, err := os.Open(path)
	if err != nil {
		return InstalledApp{}, false
	}
	defer file.Close()
	app := InstalledApp{DesktopPath: path, Size: -1}
	isFPM := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSuffix(strings.TrimSuffix(scanner.Text(), "\n"), "\r")
		switch {
		case line == "X-FPM-AppImage=true":
			isFPM = true
		case strings.HasPrefix(line, "Name="):
			app.Name = strings.TrimPrefix(line, "Name=")
		case strings.HasPrefix(line, "Categories="):
			app.Category = strings.TrimPrefix(line, "Categories=")
		case strings.HasPrefix(line, "Icon="):
			app.Icon = strings.TrimPrefix(line, "Icon=")
		case strings.HasPrefix(line, "Exec="):
			value := strings.TrimPrefix(line, "Exec=")
			if strings.HasPrefix(value, "\"") {
				if end := strings.Index(value[1:], "\""); end >= 0 {
					app.AppImagePath = value[1 : end+1]
				}
			} else {
				app.AppImagePath = strings.Fields(value)[0]
			}
		}
	}
	if !isFPM {
		return InstalledApp{}, false
	}
	base := strings.TrimSuffix(filepath.Base(path), ".desktop")
	app.Slug = base
	if app.Name == "" {
		app.Name = base
	}
	if app.AppImagePath == "" {
		app.AppImagePath = filepath.Join(appsDir, base+".AppImage")
	}
	app.Size = fileSize(app.AppImagePath)
	return app, true
}

func scanInstalled(desktopDir, appsDir string) []InstalledApp {
	entries, err := os.ReadDir(desktopDir)
	if err != nil {
		return nil
	}
	apps := make([]InstalledApp, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".desktop") {
			continue
		}
		if app, ok := parseDesktop(filepath.Join(desktopDir, entry.Name()), appsDir); ok {
			apps = append(apps, app)
		}
	}
	return apps
}

func scanAllAppImages() []InstalledApp {
	_, appsDir, _, desktopDir := appDirs()
	apps := scanInstalled(desktopDir, appsDir)
	if os.Geteuid() == 0 {
		login := os.Getenv("SUDO_USER")
		if login == "" {
			login = os.Getenv("DOAS_USER")
		}
		if login != "" {
			if account, err := user.Lookup(login); err == nil && account.HomeDir != "" {
				apps = append(apps, scanInstalled(filepath.Join(account.HomeDir, ".local", "share", "applications"), filepath.Join(account.HomeDir, ".local", "share", "appimages"))...)
			}
		}
	}
	return apps
}

func installAppImageArgs(path, name, desc, category, icon string) {
	if path == "" || fileSize(path) < 0 {
		dErr("%s", tr("ai_err_not_found"))
		return
	}
	if name == "" {
		name = guessName(path)
	}
	if desc == "" {
		desc = tr("ai_default_desc")
	}
	if category == "" {
		category = "Utility;"
	}
	if icon == "" {
		icon = "application-x-executable"
	}
	_, appsDir, iconsDir, desktopDir := appDirs()
	if err := os.MkdirAll(appsDir, 0755); err != nil {
		dErr("%s: %v", tr("ai_err_mkdir"), err)
		return
	}
	if err := os.MkdirAll(iconsDir, 0755); err != nil {
		dErr("%s: %v", tr("ai_err_mkdir"), err)
		return
	}
	if err := os.MkdirAll(desktopDir, 0755); err != nil {
		dErr("%s: %v", tr("ai_err_mkdir"), err)
		return
	}
	dHeader("%s", tr("ai_hdr_install"))
	dInfo(tr("ai_info_detected_size"), formatSize(fileSize(path)))
	slug := slugify(name)
	dest := filepath.Join(appsDir, slug+".AppImage")
	if err := copyFile(path, dest, 0755); err != nil {
		dErr("%s: %v", tr("ai_err_copy"), err)
		return
	}
	dStep(tr("ai_step_copied"), dest)
	iconValue := icon
	if iconInfo, err := os.Stat(icon); err == nil && !iconInfo.IsDir() {
		ext := filepath.Ext(icon)
		if ext == "" {
			ext = ".png"
		}
		destIcon := filepath.Join(iconsDir, slug+ext)
		if err := copyFile(icon, destIcon, 0644); err == nil {
			iconValue = destIcon
			dStep(tr("ai_step_icon_copied"), destIcon)
		}
	}
	desktop := filepath.Join(desktopDir, slug+".desktop")
	content := fmt.Sprintf("[Desktop Entry]\nType=Application\nName=%s\nComment=%s\nExec=\"%s\" %%U\nIcon=%s\nTerminal=false\nCategories=%s\nX-FPM-AppImage=true\n", name, desc, dest, iconValue, category)
	if err := os.WriteFile(desktop, []byte(content), 0644); err != nil {
		dErr("%s: %v", tr("ai_err_desktop"), err)
		return
	}
	if cmdExists("update-desktop-database") {
		runCmd([]string{"update-desktop-database", desktopDir}, true)
	}
	dStep(tr("ai_step_desktop_created"), desktop)
	dOK("%s", tr("ai_ok_installed"))
	fmt.Printf("  %s*%s %s: %s\n  %s*%s %s: %s\n  %s*%s %s: %s\n", green, reset, tr("lbl_name"), name, green, reset, tr("lbl_binary"), dest, green, reset, tr("lbl_launcher"), desktop)
}

func installAppImage(path string) {
	if path == "" || fileSize(path) < 0 {
		dErr("%s", tr("ai_err_not_found"))
		return
	}
	dInfo("%s", tr("ai_info_questions"))
	name := readLine(tr("ai_ask_name"), guessName(path))
	desc := readLine(tr("ai_ask_desc"), tr("ai_default_desc"))
	category := askCategory()
	icon := readLine(tr("ai_ask_icon"), "application-x-executable")
	installAppImageArgs(path, name, desc, category, icon)
}

func selectApp(apps []InstalledApp) int {
	for i, app := range apps {
		fmt.Printf("    %s%d)%s %s %s(%s)%s\n", cyan, i+1, reset, app.Name, dim, formatSize(app.Size), reset)
	}
	value := readLine(tr("ai_ask_select_app"), "")
	var n int
	fmt.Sscanf(value, "%d", &n)
	if n < 1 || n > len(apps) {
		dErr("%s", tr("ai_err_invalid_number"))
		return -1
	}
	return n - 1
}

func removeAppImageByName(name string) {
	apps := scanAllAppImages()
	for _, app := range apps {
		if app.Name == name || app.Slug == name {
			removeAppImage(app)
			return
		}
	}
	dErr("%s", tr("ai_info_none"))
}
func removeAppImage(app InstalledApp) {
	removed := false
	if err := os.Remove(app.AppImagePath); err == nil {
		removed = true
		dStep(tr("ai_step_removed_bin"), app.AppImagePath)
	}
	if err := os.Remove(app.DesktopPath); err == nil {
		removed = true
		dStep(tr("ai_step_removed_desktop"), app.DesktopPath)
	}
	if app.Icon != "" && filepath.IsAbs(app.Icon) {
		_ = os.Remove(app.Icon)
	}
	if removed {
		dOK("%s", tr("ai_ok_removed"))
	} else {
		dWarn("%s", tr("ai_warn_not_installed"))
	}
}
func removeAppImageInteractive() {
	apps := scanAllAppImages()
	dHeader("%s", tr("ai_hdr_remove"))
	if len(apps) == 0 {
		dInfo("%s", tr("ai_info_none"))
		return
	}
	idx := selectApp(apps)
	if idx >= 0 {
		removeAppImage(apps[idx])
	}
}

func updateAppImageByName(name, newPath string) {
	if fileSize(newPath) < 0 {
		dErr("%s", tr("ai_err_not_found"))
		return
	}
	for _, app := range scanAllAppImages() {
		if app.Name == name || app.Slug == name {
			if err := copyFile(newPath, app.AppImagePath, 0755); err != nil {
				dErr("%s", tr("ai_err_update_failed"))
			} else {
				dOK("%s", tr("ai_ok_updated"))
			}
			return
		}
	}
	dErr("%s", tr("ai_info_none"))
}
func updateAppImage(newPath string) {
	if fileSize(newPath) < 0 {
		dErr("%s", tr("ai_err_not_found"))
		return
	}
	apps := scanAllAppImages()
	dHeader("%s", tr("ai_hdr_update"))
	if len(apps) == 0 {
		dInfo("%s", tr("ai_info_none"))
		return
	}
	idx := selectApp(apps)
	if idx < 0 {
		return
	}
	if err := copyFile(newPath, apps[idx].AppImagePath, 0755); err != nil {
		dErr("%s", tr("ai_err_update_failed"))
	} else {
		dOK("%s", tr("ai_ok_updated"))
	}
}

func listAppImages() {
	apps := scanAllAppImages()
	dHeader("%s", tr("ai_hdr_list"))
	for _, app := range apps {
		fmt.Printf("  %s*%s %s %s(%s)%s\n", green, reset, app.Name, dim, formatSize(app.Size), reset)
	}
	if len(apps) == 0 {
		dInfo("%s", tr("ai_info_none"))
	}
}
func searchAppImages(query string) {
	apps := scanAllAppImages()
	matches := make([]InstalledApp, 0, len(apps))
	items := make([]TuiItem, 0, len(apps))
	for _, app := range apps {
		if query == "" || strings.Contains(strings.ToLower(app.Name), strings.ToLower(query)) || strings.Contains(strings.ToLower(app.Slug), strings.ToLower(query)) {
			matches = append(matches, app)
			items = append(items, TuiItem{Name: app.Name, Label: app.Name, Desc: app.AppImagePath, Source: "AppImage", Installed: true, Reinstallable: false, Selectable: false})
		}
	}
	if len(matches) == 0 {
		dWarn("%s", tr("ai_warn_no_search_results"))
		return
	}
	if isTTY() {
		resume := &TuiResume{}
		for {
			_, activated := selectItems("FPM AppImage - Search", items, true, resume)
			if activated == nil {
				return
			}
			manageAppByName(activated.Name)
		}
	}
	dHeader("%s: '%s'", tr("ai_hdr_search"), query)
	for _, app := range matches {
		fmt.Printf("  %s*%s %s %s(%s)%s\n", green, reset, app.Name, dim, app.AppImagePath, reset)
	}
}
func printAppImageDetails(app InstalledApp) {
	dHeader("%s", tr("ai_hdr_manage_selected"))
	fmt.Printf("  %s: %s\n", tr("lbl_name"), app.Name)
	fmt.Printf("  %s: %s\n", tr("lbl_binary"), app.AppImagePath)
	fmt.Printf("  %s: %s\n", tr("lbl_launcher"), app.DesktopPath)
	fmt.Printf("  %s: %s\n", tr("lbl_size"), formatSize(app.Size))
	if app.Category != "" {
		fmt.Printf("  %s: %s\n", tr("lbl_category"), app.Category)
	}
}

func manageSelectedApp(app InstalledApp) {
	for {
		printAppImageDetails(app)
		fmt.Println()
		fmt.Printf("  1) %s\n", tr("ai_action_update"))
		fmt.Printf("  2) %s\n", tr("ai_action_remove"))
		fmt.Printf("  3) %s\n", tr("ai_action_back"))
		action := readLine(tr("ai_ask_action"), "0")
		switch action {
		case "1":
			newPath := readLine(tr("ai_ask_new_file"), "")
			if newPath != "" {
				updateAppImageByName(app.Name, newPath)
				app.Size = fileSize(app.AppImagePath)
			}
		case "2":
			if askConfirm(tr("ai_ask_confirm_remove")) {
				removeAppImage(app)
				return
			}
		case "0", "3", "4", "":
			return
		default:
			dWarn("%s", tr("ai_err_invalid_number"))
		}
	}
}

func manageAppByName(name string) {
	for _, app := range scanAllAppImages() {
		if app.Name == name || app.Slug == name {
			manageSelectedApp(app)
			return
		}
	}
	dWarn("%s", tr("ai_warn_not_installed"))
}

func manageAppImages() {
	apps := scanAllAppImages()
	if len(apps) == 0 {
		dInfo("%s", tr("ai_info_none"))
		return
	}
	if !isTTY() {
		dErr("%s", tr("err_select_needs_tty"))
		return
	}
	idx := selectApp(apps)
	if idx >= 0 {
		manageSelectedApp(apps[idx])
	}
}
