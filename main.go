package main

import (
	"fmt"
	"os"
	"strings"
)

const version = "1.2-go"

func usage(prog string) {
	fmt.Printf("%s\n\n", tr("usage_description"))
	fmt.Printf("%s: %s <%s> [%s...]\n\n", tr("usage_label"), prog, tr("usage_command"), tr("usage_args"))
	fmt.Printf("%s:\n", tr("usage_commands_header"))
	commands := []struct{ usage, key string }{
		{"install <pkg...>", "cmd_install"}, {"remove <pkg...>", "cmd_remove"}, {"superremove <pkg...>", "cmd_superremove"},
		{"search <query>", "cmd_search"}, {"info <pkg>", "cmd_info"}, {"update", "cmd_update"}, {"upgrade", "cmd_upgrade"},
		{"list [-a|-s]", "cmd_list"}, {"repair", "cmd_repair"}, {"clean", "cmd_clean"}, {"check", "cmd_check"},
		{"files <pkg>", "cmd_files"}, {"owns <path>", "cmd_owns"}, {"deps <pkg> [-r]", "cmd_deps"}, {"hold <pkg...>", "cmd_hold"},
		{"unhold <pkg...>", "cmd_unhold"}, {"orphans", "cmd_orphans"}, {"autoremove", "cmd_autoremove"}, {"size <pkg...>", "cmd_size"}, {"diskusage", "cmd_diskusage"}, {"cache-info", "cmd_cache_info"}, {"db check", "cmd_db_check"}, {"db repair", "cmd_db_repair"},
		{"db list", "cmd_db_list"}, {"db sync", "cmd_db_sync"},
	}
	for _, command := range commands {
		fmt.Printf("  %-24s %s\n", command.usage, tr(command.key))
	}
	fmt.Printf("\n%s%s:%s\n", dim, tr("usage_appimage_header"), reset)
	fmt.Println("  aimg install [options] <file>", " ", tr("cmd_ai_install"))
	fmt.Println("  aimg remove [--name NAME] [name]", " ", tr("cmd_ai_remove"))
	fmt.Println("  aimg update [--name NAME] <file>", " ", tr("cmd_ai_update"))
	fmt.Println("  aimg list [--plain]", " ", tr("cmd_ai_list"))
	fmt.Println("  aimg search <query>", " ", tr("cmd_ai_search"))
	fmt.Println("  aimg manage       ", " ", tr("cmd_ai_manage"))
	fmt.Println("\n  AppImage options:")
	fmt.Println("    --name NAME         ", tr("ai_opt_name"))
	fmt.Println("    --desc TEXT         ", tr("ai_opt_desc"))
	fmt.Println("    --category VALUE    ", tr("ai_opt_category"))
	fmt.Println("    --icon NAME|PATH    ", tr("ai_opt_icon"))
	fmt.Println("    --plain             ", tr("ai_opt_plain"))
	fmt.Println("    --interactive       ", tr("ai_opt_interactive"))
	fmt.Printf("\n%s%s:%s\n", dim, tr("usage_flatpak_header"), reset)
	fmt.Println("  flat install <id...>", " ", tr("cmd_fp_install"))
	fmt.Println("  flat remove <id...> ", " ", tr("cmd_fp_remove"))
	fmt.Println("  flat update [id...]  ", " ", tr("cmd_fp_update"))
	fmt.Println("  flat search <query>  ", " ", tr("cmd_fp_search"))
	fmt.Println("  flat list            ", " ", tr("cmd_fp_list"))
	fmt.Println("  flat info <id>       ", " ", tr("cmd_fp_info"))
	fmt.Println("  flat remotes         ", " ", tr("cmd_fp_remotes"))
	fmt.Println("  flat remote-add <name> <url>", " ", tr("cmd_fp_remote_add"))
	fmt.Println("  flat remote-remove <name>", " ", tr("cmd_fp_remote_remove"))
	fmt.Println("  flat remote-ls [remote]", " ", tr("cmd_fp_remote_ls"))
	fmt.Println("  flat remote-info <remote> <ref>", " ", tr("cmd_fp_remote_info"))
	fmt.Println("  flat setup-flathub  ", " ", tr("cmd_fp_setup_flathub"))
	fmt.Printf("\n%sNISSA: fpm nissa v1 search <query> | fpm nissa v2 {hello|search|installed|updates|info <pkg>}%s\n", dim, reset)
	fmt.Printf("\n%s%s:\n", dim, tr("usage_options_header"))
	fmt.Println("  -y, --yes             ", tr("opt_yes"))
	fmt.Println("  -a, --all             ", tr("opt_all"))
	fmt.Println("  -s, --select          ", tr("opt_select"))
	fmt.Println("  -r, --reverse         ", tr("opt_reverse"))
	fmt.Println("  -v, --verbose         ", tr("opt_verbose"))
	fmt.Println("  -V, --version         ", tr("opt_version"))
	fmt.Println("  --remote <name>       ", tr("opt_remote"))
	fmt.Println("  --user                ", tr("opt_user"))
	fmt.Println("  -h, --help            ", tr("opt_help"))
}

func hasFlag(args []string, long, short string) bool {
	for _, arg := range args {
		if arg == long || (short != "" && arg == short) {
			return true
		}
	}
	return false
}
func positionals(args []string) []string {
	result := make([]string, 0, len(args))
	for _, arg := range args {
		if len(arg) > 0 && arg[0] != '-' {
			result = append(result, arg)
		}
	}
	return result
}

type AppImageCLIOptions struct {
	Name, Desc, Category, Icon string
	Plain, Interactive         bool
}

func parseAppImageArgs(args []string) (AppImageCLIOptions, []string, error) {
	options := AppImageCLIOptions{}
	positional := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		key, value, hasValue := strings.Cut(arg, "=")
		if !hasValue {
			key = arg
		}
		switch key {
		case "--name", "--desc", "--category", "--icon":
			if !hasValue {
				if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
					return options, positional, fmt.Errorf("%s", key)
				}
				i++
				value = args[i]
			}
			switch key {
			case "--name":
				options.Name = value
			case "--desc":
				options.Desc = value
			case "--category":
				options.Category = value
			case "--icon":
				options.Icon = value
			}
		case "--plain":
			options.Plain = true
		case "--interactive":
			options.Interactive = true
		default:
			if strings.HasPrefix(arg, "-") {
				return options, positional, fmt.Errorf("%s", arg)
			}
			positional = append(positional, arg)
		}
	}
	return options, positional, nil
}
func stringsFirst(items []string) string {
	if len(items) == 0 {
		return ""
	}
	return items[0]
}

func canonicalCommand(command string) string {
	switch command {
	case "flat":
		return "flatpak"
	case "aimg":
		return "appimage"
	default:
		return command
	}
}

func parseFlatpakCLI(args []string) (string, FlatpakOptions, []string) {
	options := FlatpakOptions{}
	positional := make([]string, 0, len(args))
	subcommand := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--user":
			options.User = true
		case "--remote":
			if i+1 < len(args) {
				options.Remote = args[i+1]
				i++
			}
		default:
			if args[i] != "" && args[i][0] != '-' && subcommand == "" {
				subcommand = args[i]
			} else if args[i] != "" && args[i][0] != '-' {
				positional = append(positional, args[i])
			}
		}
	}
	return subcommand, options, positional
}

func main() {
	initI18n()
	args := os.Args[1:]
	// Route read-only sibling calls before the ordinary CLI's global flag
	// scanner, privilege checks, terminal rendering, or TUI startup.
	if len(args) > 0 && args[0] == "nissa" {
		os.Exit(runNissa(args[1:]))
	}
	if hasFlag(args, "--verbose", "-v") {
		verbose = true
	}
	if hasFlag(args, "--yes", "-y") {
		yesFlag = true
	}
	if hasFlag(args, "--version", "-V") {
		fmt.Printf("FPM %s\n", version)
		return
	}
	if len(args) == 0 || hasFlag(args, "--help", "-h") {
		usage(os.Args[0])
		return
	}
	command := canonicalCommand(args[0])
	if command != "appimage" && command != "flatpak" {
		if command != "db" || dbCommandNeedsRoot(positionals(args[1:])) {
			elevateIfNeeded(command)
		}
		cachedDBList := command == "db" && !dbCommandNeedsRoot(positionals(args[1:]))
		if !cachedDBList && !checkXBPS() {
			return
		}
	}
	switch command {
	case "appimage":
		if len(args) < 2 {
			dErr("%s", tr("ai_err_missing_subcommand"))
			return
		}
		sub := args[1]
		options, pos, parseErr := parseAppImageArgs(args[2:])
		if parseErr != nil {
			dErr(tr("ai_err_unknown_option"), parseErr)
			return
		}
		switch sub {
		case "install":
			if len(pos) == 0 {
				dErr("%s", tr("ai_err_missing_file"))
				return
			}
			if options.Interactive || (options.Name == "" && options.Desc == "" && options.Category == "" && options.Icon == "") {
				installAppImage(pos[0])
			} else {
				installAppImageArgs(pos[0], options.Name, options.Desc, options.Category, options.Icon)
			}
		case "remove":
			target := options.Name
			if target == "" && len(pos) > 0 {
				target = pos[0]
			}
			if target != "" {
				removeAppImageByName(target)
			} else {
				removeAppImageInteractive()
			}
		case "update":
			if options.Name != "" {
				if len(pos) != 1 {
					dErr("%s", tr("ai_err_missing_file"))
					return
				}
				updateAppImageByName(options.Name, pos[0])
			} else if len(pos) > 1 {
				updateAppImageByName(pos[0], pos[1])
			} else if len(pos) == 1 {
				updateAppImage(pos[0])
			} else {
				dErr("%s", tr("ai_err_missing_file"))
			}
		case "search":
			if len(pos) == 0 {
				dErr("%s", tr("err_missing_query"))
				return
			}
			searchAppImages(pos[0])
		case "list":
			if options.Plain {

				for _, app := range scanAllAppImages() {
					fmt.Printf("%s\t%d\t%s\t%s\t%s\n", app.Name, app.Size, app.AppImagePath, app.DesktopPath, app.Category)
				}
			} else {
				listAppImages()
			}
		case "manage":
			manageAppImages()
		default:
			dErr(tr("err_unknown_command"), sub)
		}

	case "flatpak":
		if !flatpakAvailable() {
			dErr("%s", tr("fp_err_not_found"))
			return
		}
		sub, options, pos := parseFlatpakCLI(args[1:])
		if sub == "" {
			dErr("%s", tr("fp_err_missing_subcommand"))
			return
		}
		switch sub {
		case "install":
			if len(pos) == 0 {
				dErr("%s", tr("err_missing_packages"))
				return
			}
			installFlatpak(pos, options)
		case "remove":
			if len(pos) == 0 {
				dErr("%s", tr("err_missing_packages"))
				return
			}
			removeFlatpak(pos, options)
		case "update":
			updateFlatpak(pos, options)
		case "search":
			if len(pos) == 0 {
				dErr("%s", tr("err_missing_query"))
				return
			}
			searchFlatpak(pos[0], options)
		case "list":
			listFlatpak(options)
		case "info":
			if len(pos) == 0 {
				dErr("%s", tr("err_missing_package"))
				return
			}
			infoFlatpak(pos[0], options)
		case "remotes", "remote-list":
			listFlatpakRemotes(options)
		case "remote-add":
			if len(pos) < 2 {
				dErr("%s", tr("fp_err_remote_args"))
				return
			}
			addFlatpakRemote(pos[0], pos[1], options)
		case "remote-remove", "remote-delete":
			if len(pos) < 1 {
				dErr("%s", tr("fp_err_remote_args"))
				return
			}
			removeFlatpakRemote(pos[0], options)
		case "remote-info":
			if len(pos) < 2 {
				dErr("%s", tr("fp_err_remote_args"))
				return
			}
			infoFlatpakRemote(pos[0], pos[1], options)
		case "remote-ls":
			remote := ""
			if len(pos) > 0 {
				remote = pos[0]
			}
			listFlatpakRemote(remote, options)
		case "setup-flathub":
			setupFlathub(options)
		default:
			dErr(tr("err_unknown_command"), sub)
		}
	case "install":
		if p := positionals(args[1:]); len(p) > 0 {
			installXBPS(p)
		} else {
			dErr("%s", tr("err_missing_packages"))
		}
	case "remove":
		if p := positionals(args[1:]); len(p) > 0 {
			removeXBPS(p)
		} else {
			dErr("%s", tr("err_missing_packages"))
		}
	case "superremove":
		if p := positionals(args[1:]); len(p) > 0 {
			superRemove(p)
		} else {
			dErr("%s", tr("err_missing_packages"))
		}
	case "search":
		if p := positionals(args[1:]); len(p) > 0 {
			searchAll(p[0])
		} else {
			dErr("%s", tr("err_missing_query"))
		}
	case "info":
		if p := positionals(args[1:]); len(p) > 0 {
			infoXBPS(p[0])
		} else {
			dErr("%s", tr("err_missing_package"))
		}
	case "update":
		updateXBPS()
	case "upgrade":
		upgradeXBPS()
	case "list":
		listXBPS(hasFlag(args, "--all", "-a"), hasFlag(args, "--select", "-s"))
	case "repair":
		repairXBPS()
	case "clean":
		cleanXBPS()
	case "check":
		checkXBPSIntegrity()
	case "files":
		if p := positionals(args[1:]); len(p) > 0 {
			filesXBPS(p[0])
		} else {
			dErr("%s", tr("err_missing_package"))
		}
	case "owns":
		if p := positionals(args[1:]); len(p) > 0 {
			ownsXBPS(p[0])
		} else {
			dErr("%s", tr("err_missing_path"))
		}
	case "deps":
		if p := positionals(args[1:]); len(p) > 0 {
			depsXBPS(p[0], hasFlag(args, "--reverse", "-r"))
		} else {
			dErr("%s", tr("err_missing_package"))
		}
	case "hold":
		if p := positionals(args[1:]); len(p) > 0 {
			holdXBPS(p, false)
		} else {
			dErr("%s", tr("err_missing_packages"))
		}
	case "unhold":
		if p := positionals(args[1:]); len(p) > 0 {
			holdXBPS(p, true)
		} else {
			dErr("%s", tr("err_missing_packages"))
		}
	case "orphans":
		orphansXBPS(false)
	case "autoremove":
		orphansXBPS(true)
	case "size":
		if p := positionals(args[1:]); len(p) > 0 {
			sizeXBPS(p)
		} else {
			dErr("%s", tr("err_missing_packages"))
		}
	case "diskusage":
		diskUsage()
	case "cache-info":
		cacheInfo()
	case "db":
		pos := positionals(args[1:])
		if len(pos) == 0 {
			dErr("%s", tr("err_db_subcommand"))
			return
		}
		switch pos[0] {
		case "check":
			checkXBPSDatabase()
		case "repair":
			repairXBPSDatabase()
		case "list":
			if err := listFPMDatabase(); err != nil {
				fmt.Fprintf(os.Stderr, "FPM: %v\n", err)
				os.Exit(1)
			}
		case "sync":
			count, err := refreshFPMDatabase()
			if err != nil {
				fmt.Fprintf(os.Stderr, "FPM: could not sync package database: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf(tr("ok_fpm_db_synced")+"\n", count, fpmDBPath)
		default:
			dErr(tr("err_db_subcommand"))
		}
	default:
		dErr(tr("err_unknown_command"), command)
	}
}
