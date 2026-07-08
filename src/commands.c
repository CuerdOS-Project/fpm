/* SPDX-License-Identifier: GPL-3.0-or-later */
#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "commands.h"
#include "display.h"
#include "utils.h"
#include "tui.h"
#include "i18n.h"

#define MAX_ARGV 256

void fpm_check_available(void) {
    if (!cmd_exists("xbps-install")) {
        d_err("%s", t("err_xbps_not_found"));
        exit(1);
    }
}

static void join_names(char **items, int count, char *out, size_t out_size) {
    out[0] = '\0';
    for (int i = 0; i < count; i++) {
        strncat(out, items[i], out_size - strlen(out) - 1);
        if (i != count - 1) strncat(out, " ", out_size - strlen(out) - 1);
    }
}

static int ask_confirm(const char *question) {
    if (g_yes) return 1;
    printf("%s?%s %s [s/N] ", C_BYELLOW, C_RESET, question);
    fflush(stdout);
    char line[16];
    if (!fgets(line, sizeof(line), stdin)) return 0;
    return line[0] == 's' || line[0] == 'S' || line[0] == 'y' || line[0] == 'Y';
}

void fpm_install(char **packages, int count) {
    char joined[1024];
    join_names(packages, count, joined, sizeof(joined));
    char header[1100];
    snprintf(header, sizeof(header), "%s: %s", t("hdr_installing"), joined);
    d_header(header);

    char *argv[MAX_ARGV];
    int n = 0;
    argv[n++] = "xbps-install";
    argv[n++] = "-Sy";
    for (int i = 0; i < count && n < MAX_ARGV - 1; i++) argv[n++] = packages[i];
    argv[n] = NULL;

    RunResult r = run_cmd(argv, 0);
    if (r.exit_code == 0) d_ok("%s", t("ok_installed"));
    else d_err("%s", t("err_install_failed"));
}

void fpm_remove(char **packages, int count) {
    char joined[1024];
    join_names(packages, count, joined, sizeof(joined));
    char header[1100];
    snprintf(header, sizeof(header), "%s: %s", t("hdr_removing"), joined);
    d_header(header);

    char *argv[MAX_ARGV];
    int n = 0;
    argv[n++] = "xbps-remove";
    argv[n++] = "-Ry";
    for (int i = 0; i < count && n < MAX_ARGV - 1; i++) argv[n++] = packages[i];
    argv[n] = NULL;

    run_cmd(argv, 0);
    d_ok("%s", t("ok_removed"));
}

void fpm_superremove(char **packages, int count) {
    char joined[1024];
    join_names(packages, count, joined, sizeof(joined));
    char header[1100];
    snprintf(header, sizeof(header), "Superremove: %s", joined);
    d_header(header);

    d_step("%s", t("step_removing_config"));
    char *argv[MAX_ARGV];
    int n = 0;
    argv[n++] = "xbps-remove";
    argv[n++] = "-RFy";
    for (int i = 0; i < count && n < MAX_ARGV - 1; i++) argv[n++] = packages[i];
    argv[n] = NULL;
    run_cmd(argv, 0);

    d_step("%s", t("step_removing_orphans"));
    char *orphan_argv[] = { "xbps-remove", "-oy", NULL };
    run_cmd(orphan_argv, 0);

    d_step("%s", t("step_cleaning_cache"));
    char *clean_argv[] = { "xbps-remove", "-Oy", NULL };
    run_cmd(clean_argv, 0);

    d_ok("%s", t("ok_superremove_done"));
}

void fpm_update(void) {
    d_header(t("hdr_syncing"));
    char *argv[] = { "xbps-install", "-Sy", NULL };
    RunResult r = run_cmd(argv, 0);
    if (r.exit_code == 0) d_ok("%s", t("ok_synced"));
    else d_err("%s", t("err_sync_failed"));
}

void fpm_upgrade(void) {
    d_header(t("hdr_upgrading"));
    char *argv[] = { "xbps-install", "-Suy", NULL };
    run_cmd(argv, 0);
    d_ok("%s", t("ok_upgraded"));
}

void fpm_search(const char *query) {
    char header[300];
    snprintf(header, sizeof(header), "%s: '%s'", t("hdr_searching"), query);
    d_header(header);

    char *argv[] = { "xbps-query", "-Rs", (char *)query, NULL };
    RunResult r = run_cmd(argv, 1);
    if (!r.stdout_data || r.stdout_data[0] == '\0') {
        d_warn("%s", t("warn_no_packages_found"));
        free(r.stdout_data);
        return;
    }

    /* Parsea cada línea "[*] pkgname-1.0_1  Descripción..." */
    TuiItem items[512];
    int item_count = 0;
    char *saveptr = NULL;
    char *line = strtok_r(r.stdout_data, "\n", &saveptr);
    while (line && item_count < 512) {
        char *body = line;
        char *bracket = strchr(line, ']');
        if (line[0] == '[' && bracket) body = bracket + 1;
        while (*body == ' ') body++;
        char *space = strchr(body, ' ');
        char *name, *desc = NULL;
        if (space) {
            *space = '\0';
            name = body;
            desc = space + 1;
            while (*desc == ' ') desc++;
        } else {
            name = body;
        }
        if (*name) {
            items[item_count].name = strdup(name);
            items[item_count].desc = desc ? strdup(desc) : NULL;
            item_count++;
        }
        line = strtok_r(NULL, "\n", &saveptr);
    }

    if (is_tty()) {
        d_info(t("info_search_hint"));
        int sel_count = 0;
        char **selected = tui_select(items, item_count, "FPM - " , &sel_count);
        (void)selected;
        if (sel_count == 0) {
            d_info("%s", t("info_none_selected"));
        } else {
            char joined[1024];
            join_names(selected, sel_count, joined, sizeof(joined));
            printf("\n%s%s:%s %s\n", C_BOLD, t("lbl_selected"), C_RESET, joined);
            if (ask_confirm(t("ask_install_selected"))) {
                fpm_install(selected, sel_count);
            } else {
                d_info("%s", t("info_install_cancelled"));
            }
            free(selected);
        }
    } else {
        for (int i = 0; i < item_count; i++) {
            printf("  %s*%s %s  %s\n", C_BGREEN, C_RESET, items[i].name,
                   items[i].desc ? items[i].desc : "");
        }
    }

    for (int i = 0; i < item_count; i++) {
        free(items[i].name);
        free(items[i].desc);
    }
    free(r.stdout_data);
}

void fpm_info(const char *package) {
    char *argv[] = { "xbps-query", "-R", (char *)package, NULL };
    RunResult r = run_cmd(argv, 1);
    if (r.exit_code != 0 || !r.stdout_data || r.stdout_data[0] == '\0') {
        d_err(t("err_package_not_found"), package);
        d_info(t("info_try_search"), package);
        free(r.stdout_data);
        return;
    }
    char header[300];
    snprintf(header, sizeof(header), "%s: %s", t("hdr_info"), package);
    d_header(header);
    printf("%s\n", r.stdout_data);
    free(r.stdout_data);
}

void fpm_list(int show_all, int select) {
    char *argv[] = { "xbps-query", "-l", NULL };
    RunResult r = run_cmd(argv, 1);
    if (!r.stdout_data) return;

    TuiItem items[4096];
    int item_count = 0;
    char *saveptr = NULL;
    char *line = strtok_r(r.stdout_data, "\n", &saveptr);
    while (line && item_count < 4096) {
        char *first_space = strchr(line, ' ');
        char *name = line;
        if (first_space) {
            char *rest = first_space;
            while (*rest == ' ') rest++;
            name = rest;
            char *second_space = strchr(rest, ' ');
            if (second_space) *second_space = '\0';
        }
        items[item_count].name = strdup(name);
        items[item_count].desc = NULL;
        item_count++;
        line = strtok_r(NULL, "\n", &saveptr);
    }

    if (select) {
        if (!is_tty()) {
            d_err("%s", t("err_select_needs_tty"));
            goto cleanup;
        }
        d_info(t("info_list_select_hint"), item_count);
        int sel_count = 0;
        char **selected = tui_select(items, item_count, "FPM", &sel_count);
        if (sel_count == 0) {
            d_info("%s", t("info_none_selected"));
        } else {
            char joined[1024];
            join_names(selected, sel_count, joined, sizeof(joined));
            printf("\n%s%s:%s %s\n", C_BOLD, t("lbl_selected"), C_RESET, joined);
            if (ask_confirm(t("ask_remove_selected"))) {
                fpm_remove(selected, sel_count);
            } else {
                d_info("%s", t("info_remove_cancelled"));
            }
            free(selected);
        }
        goto cleanup;
    }

    char header[100];
    snprintf(header, sizeof(header), "%s (%d)", t("hdr_installed_packages"), item_count);
    d_header(header);
    int limit = show_all ? item_count : (item_count < 50 ? item_count : 50);
    for (int i = 0; i < limit; i++) {
        printf("  %s*%s %s\n", C_BGREEN, C_RESET, items[i].name);
    }
    if (!show_all && item_count > 50) {
        d_info(t("info_more_packages"), item_count - 50);
    }

cleanup:
    for (int i = 0; i < item_count; i++) free(items[i].name);
    free(r.stdout_data);
}

void fpm_repair(void) {
    d_header(t("hdr_repairing"));
    d_step("%s", t("step_reconfiguring_db"));
    char *a1[] = { "xbps-pkgdb", "-a", NULL };
    run_cmd(a1, 0);
    d_step("%s", t("step_removing_orphans"));
    char *a2[] = { "xbps-remove", "-oy", NULL };
    run_cmd(a2, 0);
    d_step("%s", t("step_cleaning_cache"));
    char *a3[] = { "xbps-remove", "-Oy", NULL };
    run_cmd(a3, 0);
    d_ok("%s", t("ok_repair_done"));
}

void fpm_clean(void) {
    d_header(t("hdr_cleaning_cache"));
    char *argv[] = { "xbps-remove", "-Oy", NULL };
    run_cmd(argv, 0);
    d_ok("%s", t("ok_cache_cleaned"));
}

void fpm_check(void) {
    d_header(t("hdr_checking_integrity"));
    char *argv[] = { "xbps-pkgdb", "-k", NULL };
    run_cmd(argv, 0);
    d_ok("%s", t("ok_check_done"));
}

void fpm_files(const char *package) {
    char *argv[] = { "xbps-query", "-f", (char *)package, NULL };
    RunResult r = run_cmd(argv, 1);
    if (r.exit_code != 0 || !r.stdout_data) {
        d_err(t("err_package_not_installed"), package);
        free(r.stdout_data);
        return;
    }
    char header[300];
    snprintf(header, sizeof(header), "%s: %s", t("hdr_files_of"), package);
    d_header(header);
    printf("%s\n", r.stdout_data);
    free(r.stdout_data);
}

void fpm_owns(const char *path) {
    char header[400];
    snprintf(header, sizeof(header), "%s: %s", t("hdr_searching_owner"), path);
    d_header(header);
    char *argv[] = { "xbps-query", "-o", (char *)path, NULL };
    RunResult r = run_cmd(argv, 1);
    if (!r.stdout_data || r.stdout_data[0] == '\0') {
        d_warn("%s", t("warn_no_owner"));
        free(r.stdout_data);
        return;
    }
    printf("%s\n", r.stdout_data);
    free(r.stdout_data);
}

void fpm_deps(const char *package, int reverse) {
    char *flag = reverse ? "-X" : "-x";
    const char *label = reverse ? t("lbl_dependents_of") : t("lbl_dependencies_of");
    char *argv[] = { "xbps-query", flag, (char *)package, NULL };
    RunResult r = run_cmd(argv, 1);
    if (r.exit_code != 0) {
        d_err(t("err_package_not_found_simple"), package);
        free(r.stdout_data);
        return;
    }
    char header[300];
    snprintf(header, sizeof(header), "%s: %s", label, package);
    d_header(header);
    if (!r.stdout_data || r.stdout_data[0] == '\0') {
        d_info("%s", t("info_no_results"));
        free(r.stdout_data);
        return;
    }
    char *saveptr = NULL;
    char *line = strtok_r(r.stdout_data, "\n", &saveptr);
    while (line) {
        if (*line) printf("  %s*%s %s\n", C_BGREEN, C_RESET, line);
        line = strtok_r(NULL, "\n", &saveptr);
    }
    free(r.stdout_data);
}

void fpm_hold(char **packages, int count, int unhold) {
    const char *mode = unhold ? "unhold" : "hold";
    const char *verb = unhold ? t("verb_unholding") : t("verb_holding");
    char joined[1024];
    join_names(packages, count, joined, sizeof(joined));
    char header[1100];
    snprintf(header, sizeof(header), "%s: %s", verb, joined);
    d_header(header);
    for (int i = 0; i < count; i++) {
        char *argv[] = { "xbps-pkgdb", "-m", (char *)mode, packages[i], NULL };
        run_cmd(argv, 0);
    }
    d_ok("%s", unhold ? t("ok_unheld") : t("ok_held"));
}

void fpm_orphans(int remove) {
    if (remove) {
        d_header(t("hdr_autoremove"));
        char *argv[] = { "xbps-remove", "-oy", NULL };
        RunResult r = run_cmd(argv, 0);
        if (r.exit_code == 0) d_ok("%s", t("ok_autoremove_done"));
        else d_warn("%s", t("warn_nothing_to_remove"));
        return;
    }

    d_header(t("hdr_orphans"));
    char *argv[] = { "xbps-remove", "-on", NULL };
    RunResult r = run_cmd(argv, 1);
    if (!r.stdout_data || r.stdout_data[0] == '\0') {
        d_info("%s", t("info_no_orphans"));
        free(r.stdout_data);
        return;
    }
    char *saveptr = NULL;
    char *line = strtok_r(r.stdout_data, "\n", &saveptr);
    int count = 0;
    while (line) {
        if (*line) { printf("  %s*%s %s\n", C_BYELLOW, C_RESET, line); count++; }
        line = strtok_r(NULL, "\n", &saveptr);
    }
    if (count > 0) d_info(t("info_orphans_hint"), count);
    free(r.stdout_data);
}

void fpm_size(char **packages, int count) {
    d_header(t("hdr_size"));
    for (int i = 0; i < count; i++) {
        char *argv[] = { "xbps-query", "-R", packages[i], NULL };
        RunResult r = run_cmd(argv, 1);
        if (r.exit_code != 0 || !r.stdout_data) {
            d_err(t("err_package_not_found_simple"), packages[i]);
            free(r.stdout_data);
            continue;
        }
        char *line = strstr(r.stdout_data, "installed_size:");
        if (line) {
            char value[128];
            sscanf(line, "installed_size: %127[^\n]", value);
            printf("  %s*%s %-24s %s\n", C_BGREEN, C_RESET, packages[i], value);
        } else {
            printf("  %s*%s %-24s %s\n", C_BYELLOW, C_RESET, packages[i], t("info_size_unknown"));
        }
        free(r.stdout_data);
    }
}

void fpm_diskusage(void) {
    d_header(t("hdr_diskusage"));
    const char *candidates[] = {
        "/var/cache/xbps",
        "/var/db/xbps",
        NULL
    };
    for (int i = 0; candidates[i]; i++) {
        char *argv[] = { "du", "-sh", (char *)candidates[i], NULL };
        RunResult r = run_cmd(argv, 1);
        if (r.exit_code == 0 && r.stdout_data && r.stdout_data[0]) {
            printf("  %s\n", r.stdout_data);
        } else {
            printf("  %s: %s\n", candidates[i], t("info_size_unknown"));
        }
        free(r.stdout_data);
    }
}
