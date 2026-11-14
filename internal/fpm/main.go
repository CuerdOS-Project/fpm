package fpm

import (
	"fmt"
	"os"
	"strings"
)

const version = "2.2"

func usage(prog string) {
	type row struct{ usage, key string }
	section := func(title string, rows []row) {
		fmt.Printf("%s%s:%s\n", dim, title, reset)
		for _, r := range rows {
			fmt.Printf("  %-24s %s\n", r.usage, tr(r.key))
		}
	}
	fmt.Printf("%s\n\n", tr("usage_description"))
	fmt.Printf("%s: %s <%s> [%s...]\n\n", tr("usage_label"), prog, tr("usage_command"), tr("usage_args"))
	section(strings.ToUpper(tr("help_hdr_system")), []row{
		{"install <pkg...>", "cmd_install"},
		{"remove <pkg...>", "cmd_remove"},
		{"superremove <pkg...>", "cmd_superremove"},
		{"search [@src] <query>", "cmd_search"},
		{"info <pkg>", "cmd_info"},
		{"update", "cmd_update"},
		{"upgrade", "cmd_upgrade"},
		{"repos", "cmd_repos"},
		{"kernel list", "cmd_kernel_list"},
		{"kernel current", "cmd_kernel_current"},
		{"kernel rm <ver...|all>", "cmd_kernel_rm"},
		{"completion [shell]", "cmd_completion"},
		{"list [-a|-s]", "cmd_list"},
		{"repair", "cmd_repair"},
		{"clean", "cmd_clean"},
		{"check", "cmd_check"},
		{"files <pkg>", "cmd_files"},
		{"owns <path>", "cmd_owns"},
		{"deps <pkg> [-r]", "cmd_deps"},
		{"hold / unhold", "cmd_hold_unhold"},
		{"orphans / autoremove", "cmd_orphans_autoremove"},
		{"size / diskusage", "cmd_size_diskusage"},
		{"cache-info", "cmd_cache_info"},
		{"db [check|repair|list|sync]", "cmd_db"},
	})
	fmt.Println()
	section(strings.ToUpper(tr("usage_appimage_header"))+" (aimg)", []row{
		{"aimg install", "cmd_ai_install"},
		{"aimg remove", "cmd_ai_remove"},
		{"aimg update", "cmd_ai_update"},
		{"aimg list", "cmd_ai_list"},
		{"aimg search <query>", "cmd_ai_search"},
		{"aimg repo", "cmd_ai_repo"},
		{"aimg info", "cmd_ai_info"},
		{"aimg manage", "cmd_ai_manage"},
	})
	fmt.Println()
	section(strings.ToUpper(tr("help_hdr_aimg_opts")), []row{
		{"--name <name>", "ai_opt_name"},
		{"--desc <text>", "ai_opt_desc"},
		{"--category <value>", "ai_opt_category"},
		{"--icon <name|path>", "ai_opt_icon"},
		{"--repo <name>", "ai_opt_repo"},
		{"--backend am|appman", "ai_opt_backend"},
		{"--plain", "ai_opt_plain"},
		{"--interactive", "ai_opt_interactive"},
		{"--sandbox", "ai_opt_passthrough"},
		{"--icons", "ai_opt_passthrough"},
		{"--debug", "ai_opt_passthrough"},
		{"--force-latest", "ai_opt_passthrough"},
	})
	fmt.Println()
	section(strings.ToUpper(tr("usage_flatpak_header"))+" (flat)", []row{
		{"flat install <id...>", "cmd_fp_install"},
		{"flat remove <id...>", "cmd_fp_remove"},
		{"flat update [id...]", "cmd_fp_update"},
		{"flat search <query>", "cmd_fp_search"},
		{"flat list", "cmd_fp_list"},
		{"flat info <id>", "cmd_fp_info"},
		{"flat remotes", "cmd_fp_remotes"},
		{"flat remote-add", "cmd_fp_remote_add"},
		{"flat remote-remove", "cmd_fp_remote_remove"},
		{"flat remote-ls", "cmd_fp_remote_ls"},
		{"flat remote-info", "cmd_fp_remote_info"},
		{"flat setup-flathub", "cmd_fp_setup_flathub"},
	})
	fmt.Println()
	section("PLUTO", []row{
		{"pluto v1 search <query>", "cmd_pluto_v1"},
		{"pluto v2", "cmd_pluto_v2"},
	})
	fmt.Println()
	section(strings.ToUpper(tr("usage_options_header")), []row{
		{"-y, --yes", "opt_yes"},
		{"-a, --all", "opt_all"},
		{"-s, --select", "opt_select"},
		{"-r, --reverse", "opt_reverse"},
		{"-v, --verbose", "opt_verbose"},
		{"-V, --version", "opt_version"},
		{"--source <src[,src]>", "opt_source"},
		{"--remote <name>", "opt_remote"},
		{"--user", "opt_user"},
		{"-h, --help", "opt_help"},
	})
}

func hasFlag(args []string, long, short string) bool {
	for _, arg := range args {
		if arg == long || (short != "" && arg == short) {
			return true
		}
	}
	return false
}

// searchQueryFromArgs arma la consulta de `fpm search` a partir de sus
// argumentos: el primer argumento sin guion es el texto; `--source xbps,flatpak`
// (o `--source=...`) y los argumentos `@fuente` limitan las fuentes. El
// resultado usa la sintaxis de itemFilter. Devuelve "" si falta el texto.
func searchQueryFromArgs(args []string) (string, error) {
	var marks []string
	text := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--source" || strings.HasPrefix(arg, "--source="):
			value := strings.TrimPrefix(arg, "--source=")
			if arg == "--source" {
				if i+1 >= len(args) {
					return "", fmt.Errorf("--source")
				}
				i++
				value = args[i]
			}
			if _, ok := parseSourceList(value); !ok {
				return "", fmt.Errorf("%s", value)
			}
			marks = append(marks, "@"+value)
		case strings.HasPrefix(arg, "@") && compileItemFilter(arg).mask != 0:
			marks = append(marks, arg)
		case len(arg) > 0 && arg[0] == '-':
			// opciones globales (-y, -v...) las procesa otro punto
		case text == "":
			text = arg
		}
	}
	if text == "" && len(marks) > 0 {
		// Solo marcas de fuente: sin texto no hay consulta.
		return "", nil
	}
	return strings.TrimSpace(strings.Join(append(marks, text), " ")), nil
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
	Name, Desc, Category, Icon, Repo, Backend string
	Plain, Interactive                        bool
	AMFlags                                   []string
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
		case "--name", "--desc", "--category", "--icon", "--repo", "--backend":
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
			case "--repo":
				options.Repo = value
			case "--backend":
				options.Backend = value
			}
		case "--user", "--sandbox", "--icons", "--debug", "--force-latest", "--assume-yes", "-y":
			options.AMFlags = append(options.AMFlags, arg)
		case "--yes":
			options.AMFlags = append(options.AMFlags, "-y")
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
	case "am":
		return "appimage"
	case "kernels":
		return "kernel"
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

// Run executes the FPM command-line application.
func Run() (exitStatus int) {
	commandFailed = false
	defer func() {
		if commandFailed && exitStatus == 0 {
			exitStatus = 1
		}
	}()
	initI18n()
	args := os.Args[1:]
	// Route read-only sibling calls before the ordinary CLI's global flag
	// scanner, privilege checks, terminal rendering, or TUI startup.
	// Shell completion helper: silent, no root, no i18n output.
	if len(args) > 0 && args[0] == "__complete" {
		os.Exit(runCompleteCommand(args[1:]))
	}
	if len(args) > 0 && args[0] == "pluto" {
		os.Exit(runPluto(args[1:]))
	}
	isAMCommand := len(args) > 0 && args[0] == "am"
	if hasFlag(args, "--verbose", "-v") {
		verbose = true
	}
	if hasFlag(args, "--yes", "-y") {
		yesFlag = true
	}
	if !isAMCommand && hasFlag(args, "--version", "-V") {
		fmt.Printf("FPM %s\n", version)
		return
	}
	if len(args) == 0 || (!isAMCommand && hasFlag(args, "--help", "-h")) {
		usage(os.Args[0])
		return
	}
	command := canonicalCommand(args[0])
	if command == "completion" {
		runCompletionCommand(positionals(args[1:]))
		return
	}
	if command != "appimage" && command != "flatpak" && command != "am" {
		if command == "repos" {
			if reposCommandNeedsRoot(positionals(args[1:])) {
				elevateIfNeeded("install")
			}
		} else if command == "kernel" {
			if kernelCommandNeedsRoot(positionals(args[1:])) {
				elevateIfNeeded("install")
			}
		} else if command != "db" || dbCommandNeedsRoot(positionals(args[1:])) {
			elevateIfNeeded(command)
		}
		cachedDBList := command == "db" && !dbCommandNeedsRoot(positionals(args[1:]))
		if !cachedDBList && !checkXBPS() {
			return
		}
	}
	switch command {
	case "appimage":
		if args[0] == "am" {
			runAMBackend(args[1:])
			return
		}
		if len(args) < 2 {
			dErr("%s", tr("ai_err_missing_subcommand"))
			return
		}
		sub := args[1]
		if sub == "repo" {
			repoArgs := args[2:]
			if len(repoArgs) == 0 {
				runAMFusionRepoCommand(nil, "")
				return
			}
			repoOptions, repoPos, parseErr := parseAppImageArgs(repoArgs[1:])
			if parseErr != nil {
				dErr(tr("ai_err_unknown_option"), parseErr)
				return
			}
			runAMFusionRepoCommand(append([]string{repoArgs[0]}, repoPos...), repoOptions.Backend)
			return
		}
		options, pos, parseErr := parseAppImageArgs(args[2:])
		if parseErr != nil {
			dErr(tr("ai_err_unknown_option"), parseErr)
			return
		}
		if options.Backend != "" {
			dErr("--backend solo se usa con `fpm aimg repo add` o `fpm aimg repo install`")
			return
		}
		switch sub {
		case "install":
			if len(pos) == 0 {
				dErr("%s", tr("ai_err_missing_file"))
				return
			}
			if options.Repo != "" || fileSize(pos[0]) < 0 {
				if err := validateAMFusionRepo(options.Repo); err != nil {
					dErr("%v", err)
					return
				}
				installAMFusionPackages(pos, options.AMFlags)
				return
			}
			if len(options.AMFlags) > 0 {
				dErr("las opciones de AM requieren `--repo %s`", amFusionRepoName)
				return
			}
			if options.Interactive || (options.Name == "" && options.Desc == "" && options.Category == "" && options.Icon == "") {
				installAppImage(pos[0])
			} else {
				installAppImageArgs(pos[0], options.Name, options.Desc, options.Category, options.Icon)
			}
		case "remove":
			if options.Repo != "" {
				if err := validateAMFusionRepoName(options.Repo); err != nil {
					dErr("%v", err)
					return
				}
				target := options.Name
				if target == "" && len(pos) > 0 {
					target = pos[0]
				}
				if target == "" {
					dErr("%s", tr("err_missing_package"))
					return
				}
				removeAMFusionPackages([]string{target}, options.AMFlags)
				return
			}
			target := options.Name
			if target == "" && len(pos) > 0 {
				target = pos[0]
			}
			if target != "" {
				if !localAppImageInstalled(target) && amBackendAvailable() {
					if err := refreshAMFusionInstalledSnapshot(); err == nil {
						if pkg, found, lookupErr := findAMFusionPackage(target); lookupErr == nil && found && pkg.Installed {
							removeAMFusionPackages([]string{pkg.Name}, options.AMFlags)
							return
						}
					}
				}
				removeAppImageByName(target)
			} else {
				removeAppImageInteractive()
			}
		case "update":
			if options.Repo != "" {
				if err := validateAMFusionRepoName(options.Repo); err != nil {
					dErr("%v", err)
					return
				}
				if options.Name != "" && len(pos) == 0 {
					pos = []string{options.Name}
				}
				updateAMFusionPackages(pos, options.AMFlags)
				return
			}
			if options.Name != "" {
				if len(pos) != 1 {
					dErr("%s", tr("ai_err_missing_file"))
					return
				}
				updateAppImageByName(options.Name, pos[0])
			} else if len(pos) > 1 {
				updateAppImageByName(pos[0], pos[1])
			} else if len(pos) == 1 {
				if fileSize(pos[0]) < 0 && amBackendAvailable() {
					if err := refreshAMFusionInstalledSnapshot(); err == nil {
						if pkg, found, lookupErr := findAMFusionPackage(pos[0]); lookupErr == nil && found && pkg.Installed {
							updateAMFusionPackages([]string{pkg.Name}, options.AMFlags)
							return
						}
					}
				}
				updateAppImage(pos[0])
			} else {
				dErr("%s", tr("ai_err_missing_file"))
			}
		case "search":
			if len(pos) == 0 {
				dErr("%s", tr("err_missing_query"))
				return
			}
			if options.Repo != "" {
				if err := validateAMFusionRepo(options.Repo); err != nil {
					dErr("%v", err)
					return
				}
				output, err := searchAMFusion(strings.Join(pos, " "))
				if err != nil {
					dErr("%v", err)
				} else {
					fmt.Print(output)
				}
				return
			}
			if enabled, err := amFusionBackendEnabled(); err == nil && enabled && amBackendAvailable() {
				if output, searchErr := searchAMFusion(strings.Join(pos, " ")); searchErr == nil {
					fmt.Print(output)
				} else {
					fmt.Fprintf(os.Stderr, "FPM: aviso: búsqueda am-FUSION no disponible: %v\n", searchErr)
				}
			}
			searchAppImages(pos[0])
		case "list":
			if options.Repo != "" {
				if err := validateAMFusionRepoName(options.Repo); err != nil {
					dErr("%v", err)
					return
				}
				if err := printAMFusionInstalled(options.Plain); err != nil {
					dErr("%v", err)
				}
				return
			}
			if options.Plain {
				refreshFPMLocalAppImageDB()
				for _, app := range scanAllAppImages() {
					fmt.Printf("%s\t%d\t%s\t%s\t%s\n", app.Name, app.Size, app.AppImagePath, app.DesktopPath, app.Category)
				}
			} else {
				listAppImages()
			}
			if amBackendAvailable() {
				if err := printAMFusionInstalled(options.Plain); err != nil {
					fmt.Fprintf(os.Stderr, "FPM: aviso: no se pudo actualizar el inventario am-FUSION: %v\n", err)
				}
			}
		case "info":
			if len(pos) == 0 {
				dErr("%s", tr("err_missing_package"))
				return
			}
			if err := validateAMFusionRepoName(options.Repo); err != nil {
				dErr("%v", err)
				return
			}
			showAMFusionInfo(pos[0])
		case "manage":
			if options.Repo != "" {
				if err := validateAMFusionRepoName(options.Repo); err != nil {
					dErr("%v", err)
					return
				}
				if err := printAMFusionInstalled(options.Plain); err != nil {
					dErr("%v", err)
				}
			} else {
				manageAppImages()
			}
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
		query, err := searchQueryFromArgs(args[1:])
		if err != nil {
			dErr(tr("err_unknown_source"), err.Error())
		} else if query == "" {
			dErr("%s", tr("err_missing_query"))
		} else {
			searchAll(query)
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
	case "repos":
		runReposCommand(positionals(args[1:]))
	case "kernel":
		runKernelCommand(positionals(args[1:]))
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
	return exitStatus
}
