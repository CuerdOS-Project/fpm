/*
 * FPM - Front-end para xbps
 * Licencia: GPL 3.0
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "display.h"
#include "utils.h"
#include "commands.h"
#include "appimage.h"
#include "flatpak.h"
#include "i18n.h"

#define VERSION "1.0-stable"
#define MAX_ARGV 256

static void print_usage(const char *prog) {
    printf("%s\n\n", t("usage_description"));
    printf("%s: %s <%s> [%s...]\n\n", t("usage_label"), prog, t("usage_command"), t("usage_args"));
    printf("%s:\n", t("usage_commands_header"));
    printf("  install <pkg...>       %s\n", t("cmd_install"));
    printf("  remove <pkg...>        %s\n", t("cmd_remove"));
    printf("  superremove <pkg...>   %s\n", t("cmd_superremove"));
    printf("  search <query>         %s\n", t("cmd_search"));
    printf("  info <pkg>             %s\n", t("cmd_info"));
    printf("  update                  %s\n", t("cmd_update"));
    printf("  upgrade                 %s\n", t("cmd_upgrade"));
    printf("  list [-a|-s]            %s\n", t("cmd_list"));
    printf("  repair                  %s\n", t("cmd_repair"));
    printf("  clean                   %s\n", t("cmd_clean"));
    printf("  check                   %s\n", t("cmd_check"));
    printf("  files <pkg>             %s\n", t("cmd_files"));
    printf("  owns <path>             %s\n", t("cmd_owns"));
    printf("  deps <pkg> [-r]         %s\n", t("cmd_deps"));
    printf("  hold <pkg...>           %s\n", t("cmd_hold"));
    printf("  unhold <pkg...>         %s\n", t("cmd_unhold"));
    printf("  orphans                 %s\n", t("cmd_orphans"));
    printf("  autoremove              %s\n", t("cmd_autoremove"));
    printf("  size <pkg...>           %s\n", t("cmd_size"));
    printf("  diskusage               %s\n", t("cmd_diskusage"));
    printf("\n%s%s:%s\n", C_DIM, t("usage_appimage_header"), C_RESET);
    printf("  appimage install <file> %s\n", t("cmd_ai_install"));
    printf("  appimage remove         %s\n", t("cmd_ai_remove"));
    printf("  appimage update <file>  %s\n", t("cmd_ai_update"));
    printf("  appimage list           %s\n", t("cmd_ai_list"));
    printf("  appimage manage         %s\n", t("cmd_ai_manage"));
    printf("\n%s%s:%s\n", C_DIM, t("usage_flatpak_header"), C_RESET);
    printf("  flatpak install <id...> %s\n", t("cmd_fp_install"));
    printf("  flatpak remove <id...>  %s\n", t("cmd_fp_remove"));
    printf("  flatpak update [id...]  %s\n", t("cmd_fp_update"));
    printf("  flatpak search <query>  %s\n", t("cmd_fp_search"));
    printf("  flatpak list            %s\n", t("cmd_fp_list"));
    printf("  flatpak info <id>       %s\n", t("cmd_fp_info"));
    printf("\n%s%s:\n", C_DIM, t("usage_options_header"));
    printf("  -y, --yes             %s\n", t("opt_yes"));
    printf("  -a, --all             %s\n", t("opt_all"));
    printf("  -s, --select          %s\n", t("opt_select"));
    printf("  -r, --reverse         %s\n", t("opt_reverse"));
    printf("  -v, --verbose         %s\n", t("opt_verbose"));
    printf("  -V, --version         %s\n", t("opt_version"));
    printf("  -h, --help            %s\n", t("opt_help"));
    printf("\n%s%s:\n", C_DIM, t("usage_examples_header"));
    printf("  %s install neofetch htop\n", prog);
    printf("  %s search htop\n", prog);
    printf("  %s list --select\n", prog);
    printf("  %s remove htop -y\n", prog);
    printf("  %s orphans\n", prog);
    printf("  %s size firefox\n", prog);
    printf("  %s appimage install ~/Descargas/MiApp.AppImage\n", prog);
    printf("  %s appimage update ~/Descargas/MiApp-2.0.AppImage\n", prog);
    printf("  %s appimage manage\n", prog);
    printf("  %s flatpak search gimp\n", prog);
    printf("  %s flatpak install org.gimp.GIMP%s\n", prog, C_RESET);
}

/* Busca 'flag' (forma larga) o 'short_flag' (forma corta, ej "-y"). */
static int find_flag(int argc, char *argv[], const char *flag, const char *short_flag) {
    for (int i = 1; i < argc; i++) {
        if (strcmp(argv[i], flag) == 0) return 1;
        if (short_flag && strcmp(argv[i], short_flag) == 0) return 1;
    }
    return 0;
}

/* Recolecta los argumentos posicionales, ignorando los que empiezan
 * con "-" (banderas largas "--foo" y cortas "-f"). */
static int collect_positional(int argc, char *argv[], int start, char **out) {
    int n = 0;
    for (int i = start; i < argc; i++) {
        if (argv[i][0] != '-') out[n++] = argv[i];
    }
    return n;
}

int main(int argc, char *argv[]) {
    i18n_init();

    if (find_flag(argc, argv, "--verbose", "-v")) g_verbose = 1;

    if (find_flag(argc, argv, "--version", "-V")) {
        printf("FPM %s\n", VERSION);
        return 0;
    }

    if (argc < 2) {
        print_usage(argv[0]);
        return 0;
    }

    const char *command = argv[1];

    if (strcmp(command, "help") == 0 || strcmp(command, "--help") == 0 ||
        strcmp(command, "-h") == 0) {
        print_usage(argv[0]);
        i18n_shutdown();
        return 0;
    }

    int is_appimage_cmd = strcmp(command, "appimage") == 0;
    int is_flatpak_cmd = strcmp(command, "flatpak") == 0;

    if (!is_appimage_cmd && !is_flatpak_cmd) {
        elevate_if_needed(command, argc, argv);
        fpm_check_available();
    }

    if (find_flag(argc, argv, "--yes", "-y")) g_yes = 1;

    char *positional[MAX_ARGV];
    int pcount = collect_positional(argc, argv, 2, positional);

    /* --- appimage <subcomando> [args...] --- */
    if (is_appimage_cmd) {
        const char *sub = argc > 2 ? argv[2] : NULL;
        char *sub_positional[MAX_ARGV];
        int sub_pcount = collect_positional(argc, argv, 3, sub_positional);

        if (!sub) {
            d_err("%s", t("ai_err_missing_subcommand"));
            i18n_shutdown();
            return 1;
        } else if (strcmp(sub, "install") == 0) {
            if (sub_pcount < 1) { d_err("%s", t("ai_err_missing_file")); i18n_shutdown(); return 1; }
            const char *name = NULL, *desc = NULL, *category = NULL, *icon = NULL;
            for (int i = 3; i < argc; i++) {
                if (strcmp(argv[i], "--name") == 0 && i + 1 < argc) name = argv[++i];
                else if (strcmp(argv[i], "--desc") == 0 && i + 1 < argc) desc = argv[++i];
                else if (strcmp(argv[i], "--category") == 0 && i + 1 < argc) category = argv[++i];
                else if (strcmp(argv[i], "--icon") == 0 && i + 1 < argc) icon = argv[++i];
            }
            if (name) fpm_appimage_install_args(sub_positional[0], name, desc, category, icon);
            else fpm_appimage_install(sub_positional[0]);
        } else if (strcmp(sub, "remove") == 0) {
            if (sub_pcount >= 1) fpm_appimage_remove_by_name(sub_positional[0]);
            else fpm_appimage_remove();
        } else if (strcmp(sub, "update") == 0) {
            if (sub_pcount < 1) { d_err("%s", t("ai_err_missing_file")); i18n_shutdown(); return 1; }
            if (sub_pcount >= 2) fpm_appimage_update_by_name(sub_positional[0], sub_positional[1]);
            else fpm_appimage_update(sub_positional[0]);
        } else if (strcmp(sub, "list") == 0) {
            if (find_flag(argc, argv, "--plain", NULL)) fpm_appimage_list_plain();
            else fpm_appimage_list();
        } else if (strcmp(sub, "manage") == 0) {
            fpm_appimage_manage();
        } else {
            d_err(t("err_unknown_command"), sub);
            i18n_shutdown();
            return 1;
        }
        i18n_shutdown();
        return 0;
    }

    /* --- flatpak <subcomando> [args...] --- */
    if (is_flatpak_cmd) {
        const char *sub = argc > 2 ? argv[2] : NULL;
        char *sub_positional[MAX_ARGV];
        int sub_pcount = collect_positional(argc, argv, 3, sub_positional);

        if (!fpm_flatpak_available()) {
            d_err("%s", t("fp_err_not_found"));
            i18n_shutdown();
            return 1;
        }

        if (!sub) {
            d_err("%s", t("fp_err_missing_subcommand"));
            i18n_shutdown();
            return 1;
        } else if (strcmp(sub, "install") == 0) {
            if (sub_pcount == 0) { d_err("%s", t("err_missing_packages")); i18n_shutdown(); return 1; }
            fpm_flatpak_install(sub_positional, sub_pcount);
        } else if (strcmp(sub, "remove") == 0) {
            if (sub_pcount == 0) { d_err("%s", t("err_missing_packages")); i18n_shutdown(); return 1; }
            fpm_flatpak_remove(sub_positional, sub_pcount);
        } else if (strcmp(sub, "update") == 0) {
            fpm_flatpak_update(sub_positional, sub_pcount);
        } else if (strcmp(sub, "search") == 0) {
            if (sub_pcount < 1) { d_err("%s", t("err_missing_query")); i18n_shutdown(); return 1; }
            fpm_flatpak_search(sub_positional[0]);
        } else if (strcmp(sub, "list") == 0) {
            fpm_flatpak_list();
        } else if (strcmp(sub, "info") == 0) {
            if (sub_pcount < 1) { d_err("%s", t("err_missing_package")); i18n_shutdown(); return 1; }
            fpm_flatpak_info(sub_positional[0]);
        } else {
            d_err(t("err_unknown_command"), sub);
            i18n_shutdown();
            return 1;
        }
        i18n_shutdown();
        return 0;
    }

    if (strcmp(command, "install") == 0) {
        if (pcount == 0) { d_err("%s", t("err_missing_packages")); return 1; }
        fpm_install(positional, pcount);
    } else if (strcmp(command, "remove") == 0) {
        if (pcount == 0) { d_err("%s", t("err_missing_packages")); return 1; }
        fpm_remove(positional, pcount);
    } else if (strcmp(command, "superremove") == 0) {
        if (pcount == 0) { d_err("%s", t("err_missing_packages")); return 1; }
        fpm_superremove(positional, pcount);
    } else if (strcmp(command, "search") == 0) {
        if (pcount < 1) { d_err("%s", t("err_missing_query")); return 1; }
        fpm_search(positional[0]);
    } else if (strcmp(command, "info") == 0) {
        if (pcount < 1) { d_err("%s", t("err_missing_package")); return 1; }
        fpm_info(positional[0]);
    } else if (strcmp(command, "update") == 0) {
        fpm_update();
    } else if (strcmp(command, "upgrade") == 0) {
        fpm_upgrade();
    } else if (strcmp(command, "list") == 0) {
        fpm_list(find_flag(argc, argv, "--all", "-a"), find_flag(argc, argv, "--select", "-s"));
    } else if (strcmp(command, "repair") == 0) {
        fpm_repair();
    } else if (strcmp(command, "clean") == 0) {
        fpm_clean();
    } else if (strcmp(command, "check") == 0) {
        fpm_check();
    } else if (strcmp(command, "files") == 0) {
        if (pcount < 1) { d_err("%s", t("err_missing_package")); return 1; }
        fpm_files(positional[0]);
    } else if (strcmp(command, "owns") == 0) {
        if (pcount < 1) { d_err("%s", t("err_missing_path")); return 1; }
        fpm_owns(positional[0]);
    } else if (strcmp(command, "deps") == 0) {
        if (pcount < 1) { d_err("%s", t("err_missing_package")); return 1; }
        fpm_deps(positional[0], find_flag(argc, argv, "--reverse", "-r"));
    } else if (strcmp(command, "hold") == 0) {
        if (pcount == 0) { d_err("%s", t("err_missing_packages")); return 1; }
        fpm_hold(positional, pcount, 0);
    } else if (strcmp(command, "unhold") == 0) {
        if (pcount == 0) { d_err("%s", t("err_missing_packages")); return 1; }
        fpm_hold(positional, pcount, 1);
    } else if (strcmp(command, "orphans") == 0) {
        fpm_orphans(0);
    } else if (strcmp(command, "autoremove") == 0) {
        fpm_orphans(1);
    } else if (strcmp(command, "size") == 0) {
        if (pcount == 0) { d_err("%s", t("err_missing_packages")); return 1; }
        fpm_size(positional, pcount);
    } else if (strcmp(command, "diskusage") == 0) {
        fpm_diskusage();
    } else {
        d_err(t("err_unknown_command"), command);
        return 1;
    }

    i18n_shutdown();
    return 0;
}
