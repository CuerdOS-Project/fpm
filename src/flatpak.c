/* SPDX-License-Identifier: GPL-3.0-or-later */
#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "flatpak.h"
#include "display.h"
#include "utils.h"
#include "tui.h"
#include "i18n.h"

#define MAX_ARGV 256

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

int fpm_flatpak_available(void) {
    return cmd_exists("flatpak");
}

static int check_or_warn(void) {
    if (!fpm_flatpak_available()) {
        d_err("%s", t("fp_err_not_found"));
        return 0;
    }
    return 1;
}

void fpm_flatpak_install(char **packages, int count) {
    if (!check_or_warn()) return;

    char joined[1024];
    join_names(packages, count, joined, sizeof(joined));
    char header[1100];
    snprintf(header, sizeof(header), "%s: %s", t("fp_hdr_installing"), joined);
    d_header(header);

    char *argv[MAX_ARGV];
    int n = 0;
    argv[n++] = "flatpak";
    argv[n++] = "install";
    argv[n++] = "-y";
    argv[n++] = "flathub";
    for (int i = 0; i < count && n < MAX_ARGV - 1; i++) argv[n++] = packages[i];
    argv[n] = NULL;

    RunResult r = run_cmd(argv, 0);
    if (r.exit_code == 0) d_ok("%s", t("fp_ok_installed"));
    else d_err("%s", t("fp_err_install_failed"));
}

void fpm_flatpak_remove(char **packages, int count) {
    if (!check_or_warn()) return;

    char joined[1024];
    join_names(packages, count, joined, sizeof(joined));
    char header[1100];
    snprintf(header, sizeof(header), "%s: %s", t("fp_hdr_removing"), joined);
    d_header(header);

    char *argv[MAX_ARGV];
    int n = 0;
    argv[n++] = "flatpak";
    argv[n++] = "uninstall";
    argv[n++] = "-y";
    for (int i = 0; i < count && n < MAX_ARGV - 1; i++) argv[n++] = packages[i];
    argv[n] = NULL;

    RunResult r = run_cmd(argv, 0);
    if (r.exit_code == 0) d_ok("%s", t("fp_ok_removed"));
    else d_err("%s", t("fp_err_remove_failed"));
}

void fpm_flatpak_update(char **packages, int count) {
    if (!check_or_warn()) return;

    d_header(count == 0 ? t("fp_hdr_updating_all") : t("fp_hdr_updating"));

    char *argv[MAX_ARGV];
    int n = 0;
    argv[n++] = "flatpak";
    argv[n++] = "update";
    argv[n++] = "-y";
    for (int i = 0; i < count && n < MAX_ARGV - 1; i++) argv[n++] = packages[i];
    argv[n] = NULL;

    RunResult r = run_cmd(argv, 0);
    if (r.exit_code == 0) d_ok("%s", t("fp_ok_updated"));
    else d_err("%s", t("fp_err_update_failed"));
}

void fpm_flatpak_search(const char *query) {
    if (!check_or_warn()) return;

    char header[300];
    snprintf(header, sizeof(header), "%s: '%s'", t("fp_hdr_searching"), query);
    d_header(header);

    /* flatpak search --columns=name,description,application <query> */
    char *argv[] = { "flatpak", "search", "--columns=name,description,application",
                      (char *)query, NULL };
    RunResult r = run_cmd(argv, 1);
    if (!r.stdout_data || r.stdout_data[0] == '\0') {
        d_warn("%s", t("fp_warn_no_packages_found"));
        free(r.stdout_data);
        return;
    }

    /* Cada línea: "Nombre\tDescripción\tapp.id" */
    TuiItem items[512];
    char *app_ids[512];
    int item_count = 0;
    char *saveptr = NULL;
    char *line = strtok_r(r.stdout_data, "\n", &saveptr);
    while (line && item_count < 512) {
        char *name = line;
        char *tab1 = strchr(line, '\t');
        char *desc = NULL;
        char *app_id = name;
        if (tab1) {
            *tab1 = '\0';
            desc = tab1 + 1;
            char *tab2 = strchr(desc, '\t');
            if (tab2) {
                *tab2 = '\0';
                app_id = tab2 + 1;
            }
        }
        items[item_count].name = strdup(name);
        items[item_count].desc = desc ? strdup(desc) : NULL;
        app_ids[item_count] = strdup(app_id);
        item_count++;
        line = strtok_r(NULL, "\n", &saveptr);
    }

    if (is_tty()) {
        d_info(t("fp_info_search_hint"));
        int sel_count = 0;
        char **selected_names = tui_select(items, item_count, "FPM Flatpak - ", &sel_count);
        if (sel_count == 0) {
            d_info("%s", t("info_none_selected"));
        } else {
            /* Traduce nombres seleccionados a application-id reales */
            char *selected_ids[512];
            int found = 0;
            for (int s = 0; s < sel_count; s++) {
                for (int i = 0; i < item_count; i++) {
                    if (strcmp(selected_names[s], items[i].name) == 0) {
                        selected_ids[found++] = app_ids[i];
                        break;
                    }
                }
            }
            char joined[1024];
            join_names(selected_ids, found, joined, sizeof(joined));
            printf("\n%s%s:%s %s\n", C_BOLD, t("lbl_selected"), C_RESET, joined);
            if (ask_confirm(t("fp_ask_install_selected"))) {
                fpm_flatpak_install(selected_ids, found);
            } else {
                d_info("%s", t("info_install_cancelled"));
            }
            free(selected_names);
        }
    } else {
        for (int i = 0; i < item_count; i++) {
            printf("  %s*%s %s %s(%s)%s  %s\n", C_BGREEN, C_RESET, items[i].name,
                   C_DIM, app_ids[i], C_RESET, items[i].desc ? items[i].desc : "");
        }
    }

    for (int i = 0; i < item_count; i++) {
        free(items[i].name);
        free(items[i].desc);
        free(app_ids[i]);
    }
    free(r.stdout_data);
}

void fpm_flatpak_list(void) {
    if (!check_or_warn()) return;

    d_header(t("fp_hdr_list"));
    char *argv[] = { "flatpak", "list", "--columns=name,application,version", NULL };
    RunResult r = run_cmd(argv, 1);
    if (!r.stdout_data || r.stdout_data[0] == '\0') {
        d_info("%s", t("fp_info_none_installed"));
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

void fpm_flatpak_info(const char *package) {
    if (!check_or_warn()) return;

    char *argv[] = { "flatpak", "info", (char *)package, NULL };
    RunResult r = run_cmd(argv, 1);
    if (r.exit_code != 0 || !r.stdout_data || r.stdout_data[0] == '\0') {
        d_err(t("fp_err_package_not_found"), package);
        free(r.stdout_data);
        return;
    }
    char header[300];
    snprintf(header, sizeof(header), "%s: %s", t("fp_hdr_info"), package);
    d_header(header);
    printf("%s\n", r.stdout_data);
    free(r.stdout_data);
}
