/* SPDX-License-Identifier: GPL-3.0-or-later */
#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <dirent.h>
#include <limits.h>
#include <pwd.h>
#include <ctype.h>
#include <errno.h>
#include "appimage.h"
#include "appimage_backend.h"
#include "display.h"
#include "utils.h"
#include "tui.h"
#include "i18n.h"

/* Directorios de destino, resueltos en tiempo de ejecución según el
 * usuario (root instala en /opt y /usr/share, usuario normal instala
 * en ~/.local). */
static void get_dirs(char *bin_dir, size_t bin_size,
                      char *apps_dir, size_t apps_size,
                      char *icons_dir, size_t icons_size,
                      char *desktop_dir, size_t desktop_size) {
    if (geteuid() == 0) {
        snprintf(bin_dir, bin_size, "/usr/local/bin");
        snprintf(apps_dir, apps_size, "/opt/appimages");
        snprintf(icons_dir, icons_size, "/usr/local/share/icons");
        snprintf(desktop_dir, desktop_size, "/usr/local/share/applications");
        return;
    }

    const char *home = getenv("HOME");
    if (!home) {
        struct passwd *pw = getpwuid(getuid());
        home = pw ? pw->pw_dir : "/tmp";
    }
    snprintf(bin_dir, bin_size, "%s/.local/bin", home);
    snprintf(apps_dir, apps_size, "%s/.local/share/appimages", home);
    snprintf(icons_dir, icons_size, "%s/.local/share/icons", home);
    snprintf(desktop_dir, desktop_size, "%s/.local/share/applications", home);
}

static int mkdir_p(const char *path) {
    char tmp[PATH_MAX];
    snprintf(tmp, sizeof(tmp), "%s", path);
    size_t len = strlen(tmp);
    if (len == 0) return -1;
    if (tmp[len - 1] == '/') tmp[len - 1] = '\0';
    for (char *p = tmp + 1; *p; p++) {
        if (*p == '/') {
            *p = '\0';
            mkdir(tmp, 0755);
            *p = '/';
        }
    }
    return mkdir(tmp, 0755) == 0 || errno == EEXIST ? 0 : -1;
}

/* Pide una línea al usuario mostrando un valor por defecto entre
 * corchetes. Si el usuario solo presiona ENTER, se usa el valor por
 * defecto. */
static void ask_input(const char *prompt, const char *def, char *out, size_t out_size) {
    if (def && *def) {
        printf("%s%s?%s %s %s[%s]%s: ", C_BOLD, C_BYELLOW, C_RESET, prompt, C_DIM, def, C_RESET);
    } else {
        printf("%s%s?%s %s: ", C_BOLD, C_BYELLOW, C_RESET, prompt);
    }
    fflush(stdout);

    if (!fgets(out, out_size, stdin)) {
        snprintf(out, out_size, "%s", def ? def : "");
        return;
    }
    size_t n = strlen(out);
    while (n > 0 && (out[n - 1] == '\n' || out[n - 1] == '\r')) out[--n] = '\0';
    if (n == 0 && def) snprintf(out, out_size, "%s", def);
}

/* Convierte un nombre en un "slug" apto para nombres de archivo:
 * minúsculas, espacios -> guiones, sin caracteres raros. */
static void slugify(const char *in, char *out, size_t out_size) {
    size_t j = 0;
    for (size_t i = 0; in[i] && j < out_size - 1; i++) {
        char c = in[i];
        if (c >= 'A' && c <= 'Z') c = (char)(c - 'A' + 'a');
        if ((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
            out[j++] = c;
        } else if (c == ' ') {
            out[j++] = '-';
        }
    }
    out[j] = '\0';
    if (j == 0) snprintf(out, out_size, "app");
}

/* Deriva un nombre "bonito" por defecto a partir del nombre de archivo
 * del AppImage, ej: "My-App-2.3.AppImage" -> "My App". */
static void guess_name_from_path(const char *path, char *out, size_t out_size) {
    const char *base = strrchr(path, '/');
    base = base ? base + 1 : path;
    char tmp[512];
    snprintf(tmp, sizeof(tmp), "%s", base);

    /* Quita la extensión .AppImage / .appimage */
    size_t len = strlen(tmp);
    const char *ext = ".AppImage";
    size_t extlen = strlen(ext);
    if (len > extlen) {
        char lower[16];
        for (size_t i = 0; i < extlen && i < sizeof(lower) - 1; i++)
            lower[i] = (char)tolower((unsigned char)tmp[len - extlen + i]);
        lower[extlen] = '\0';
        if (strcasecmp(tmp + len - extlen, ext) == 0) tmp[len - extlen] = '\0';
    }

    /* Reemplaza guiones/underscores por espacios para que se vea legible */
    for (char *p = tmp; *p; p++) {
        if (*p == '_' || *p == '-' || *p == '.') *p = ' ';
    }
    snprintf(out, out_size, "%s", tmp);
}

static int file_exists(const char *path) {
    struct stat st;
    return stat(path, &st) == 0;
}

/* Devuelve el tamaño en bytes de 'path', o -1 si no se puede leer. */
static long get_file_size(const char *path) {
    struct stat st;
    if (stat(path, &st) != 0) return -1;
    return (long)st.st_size;
}

static int copy_file(const char *src, const char *dst, mode_t mode) {
    FILE *in = fopen(src, "rb");
    if (!in) return -1;
    FILE *out = fopen(dst, "wb");
    if (!out) { fclose(in); return -1; }

    char buf[65536];
    size_t n;
    while ((n = fread(buf, 1, sizeof(buf), in)) > 0) {
        if (fwrite(buf, 1, n, out) != n) { fclose(in); fclose(out); return -1; }
    }
    fclose(in);
    fclose(out);
    chmod(dst, mode);
    return 0;
}

/* Categorías XDG comunes que se pueden elegir presionando un número. */
static const char *CATEGORY_KEYS[] = {
    "ai_cat_utility", "ai_cat_development", "ai_cat_graphics", "ai_cat_network",
    "ai_cat_office", "ai_cat_multimedia", "ai_cat_system", "ai_cat_game",
    "ai_cat_education", "ai_cat_science", NULL
};
static const char *CATEGORY_VALUES[] = {
    "Utility;", "Development;", "Graphics;", "Network;", "Office;",
    "AudioVideo;", "System;", "Game;", "Education;", "Science;", NULL
};

/* Muestra el menú de categorías y devuelve la elegida (valor XDG listo
 * para el campo Categories= del .desktop). Por defecto, si el usuario
 * no ingresa un número válido, se usa "Utility;". */
static void ask_category(char *out, size_t out_size) {
    printf("%s%s?%s %s\n", C_BOLD, C_BYELLOW, C_RESET, t("ai_ask_category"));
    for (int i = 0; CATEGORY_KEYS[i]; i++) {
        printf("    %s%d)%s %s\n", C_BCYAN, i + 1, C_RESET, t(CATEGORY_KEYS[i]));
    }
    printf("  %s>%s ", C_BYELLOW, C_RESET);
    fflush(stdout);

    char line[16];
    int choice = 1;
    if (fgets(line, sizeof(line), stdin)) {
        int n = atoi(line);
        if (n >= 1 && n <= 10) choice = n;
    }
    snprintf(out, out_size, "%s", CATEGORY_VALUES[choice - 1]);
}

typedef struct {
    char slug[256];
    char name[256];
    char category[64];
    char icon[300];
    char desktop_path[PATH_MAX];
    char appimage_path[PATH_MAX];
    long size; /* bytes; -1 si no se pudo determinar */
} InstalledApp;

/* Copia src a dst truncando a dst_size-1 sin activar -Wformat-truncation. */
static void fpm_strlcpy(char *dst, size_t dst_size, const char *src) {
    size_t len = strlen(src);
    if (len >= dst_size) len = dst_size - 1;
    memcpy(dst, src, len);
    dst[len] = '\0';
}

/* Recorre desktop_dir buscando lanzadores creados por FPM y llena 'out'
 * con hasta 'max' entradas. Devuelve la cantidad encontrada. */
static int scan_installed(const char *desktop_dir, const char *apps_dir,
                           InstalledApp *out, int max) {
    DIR *d = opendir(desktop_dir);
    if (!d) return 0;

    int count = 0;
    struct dirent *entry;
    while ((entry = readdir(d)) != NULL && count < max) {
        const char *dot = strrchr(entry->d_name, '.');
        if (!dot || strcmp(dot, ".desktop") != 0) continue;

        char full[PATH_MAX];
        snprintf(full, sizeof(full), "%s/%s", desktop_dir, entry->d_name);
        FILE *f = fopen(full, "r");
        if (!f) continue;

        int is_fpm = 0;
        char name[256] = "";
        char exec_path[PATH_MAX] = "";
        char category[64] = "";
        char icon[300] = "";
        char line[1024];
        while (fgets(line, sizeof(line), f)) {
            size_t n = strlen(line);
            while (n > 0 && (line[n - 1] == '\n' || line[n - 1] == '\r')) line[--n] = '\0';
            if (strncmp(line, "X-FPM-AppImage=true", 19) == 0) {
                is_fpm = 1;
            } else if (strncmp(line, "Name=", 5) == 0) {
                fpm_strlcpy(name, sizeof(name), line + 5);
            } else if (strncmp(line, "Categories=", 11) == 0) {
                fpm_strlcpy(category, sizeof(category), line + 11);
            } else if (strncmp(line, "Icon=", 5) == 0) {
                fpm_strlcpy(icon, sizeof(icon), line + 5);
            } else if (strncmp(line, "Exec=", 5) == 0) {
                char *start = strchr(line + 5, '"');
                if (start) {
                    start++;
                    char *end = strchr(start, '"');
                    if (end) {
                        size_t len = (size_t)(end - start);
                        if (len >= sizeof(exec_path)) len = sizeof(exec_path) - 1;
                        memcpy(exec_path, start, len);
                        exec_path[len] = '\0';
                    }
                }
            }
        }
        fclose(f);

        if (is_fpm) {
            size_t baselen = strlen(entry->d_name) - strlen(".desktop");
            if (baselen >= sizeof(out[count].slug)) baselen = sizeof(out[count].slug) - 1;
            memcpy(out[count].slug, entry->d_name, baselen);
            out[count].slug[baselen] = '\0';

            snprintf(out[count].name, sizeof(out[count].name), "%s",
                      name[0] ? name : out[count].slug);
            snprintf(out[count].category, sizeof(out[count].category), "%s", category);
            snprintf(out[count].icon, sizeof(out[count].icon), "%s", icon);
            snprintf(out[count].desktop_path, sizeof(out[count].desktop_path), "%s", full);

            if (exec_path[0]) {
                snprintf(out[count].appimage_path, sizeof(out[count].appimage_path), "%s", exec_path);
            } else {
                snprintf(out[count].appimage_path, sizeof(out[count].appimage_path),
                          "%s/%s.AppImage", apps_dir, out[count].slug);
            }
            out[count].size = get_file_size(out[count].appimage_path);
            count++;
        }
    }
    closedir(d);
    return count;
}

/* Imprime un menú numerado de AppImages instalados y devuelve el
 * índice elegido (0-based), o -1 si la selección fue inválida o
 * cancelada. */
static int select_installed(InstalledApp *apps, int count) {
    for (int i = 0; i < count; i++) {
        char sizebuf[32];
        if (apps[i].size >= 0) format_size(apps[i].size, sizebuf, sizeof(sizebuf));
        else snprintf(sizebuf, sizeof(sizebuf), "%s", t("info_size_unknown"));
        printf("    %s%d)%s %s %s(%s)%s\n", C_BCYAN, i + 1, C_RESET, apps[i].name,
               C_DIM, sizebuf, C_RESET);
    }
    printf("%s%s?%s %s ", C_BOLD, C_BYELLOW, C_RESET, t("ai_ask_select_app"));
    fflush(stdout);

    char line[16];
    if (!fgets(line, sizeof(line), stdin)) return -1;
    int n = atoi(line);
    if (n < 1 || n > count) {
        d_err("%s", t("ai_err_invalid_number"));
        return -1;
    }
    return n - 1;
}

static void install_core(const char *path, const char *name, const char *desc,
                          const char *category, const char *icon) {
    d_header(t("ai_hdr_install"));

    long src_size = get_file_size(path);
    char src_sizebuf[32];
    if (src_size >= 0) format_size(src_size, src_sizebuf, sizeof(src_sizebuf));
    else snprintf(src_sizebuf, sizeof(src_sizebuf), "%s", t("info_size_unknown"));
    d_info(t("ai_info_detected_size"), src_sizebuf);

    char bin_dir[PATH_MAX], apps_dir[PATH_MAX], icons_dir[PATH_MAX], desktop_dir[PATH_MAX];
    get_dirs(bin_dir, sizeof(bin_dir), apps_dir, sizeof(apps_dir),
              icons_dir, sizeof(icons_dir), desktop_dir, sizeof(desktop_dir));

    char slug[256];
    slugify(name, slug, sizeof(slug));

    if (mkdir_p(apps_dir) != 0 || mkdir_p(bin_dir) != 0 ||
        mkdir_p(desktop_dir) != 0 || mkdir_p(icons_dir) != 0) {
        d_err("%s", t("ai_err_mkdir"));
        return;
    }

    /* Copia el AppImage a su ubicación final.
     * Nota: apps_dir y slug están acotados en la práctica muy por debajo
     * de PATH_MAX; se ignora el falso positivo de -Wformat-truncation. */
    char dest_appimage[PATH_MAX];
#pragma GCC diagnostic push
#pragma GCC diagnostic ignored "-Wformat-truncation"
    snprintf(dest_appimage, sizeof(dest_appimage), "%s/%s.AppImage", apps_dir, slug);
    if (copy_file(path, dest_appimage, 0755) != 0) {
        d_err("%s", t("ai_err_copy"));
        return;
    }
#pragma GCC diagnostic pop
    d_step(t("ai_step_copied"), dest_appimage);

    /* Si el ícono es una ruta a un archivo existente (no un nombre de
     * tema de íconos), lo copiamos junto a los recursos de la app y
     * usamos esa ruta absoluta en el .desktop. */
    char icon_value[PATH_MAX];
    if (file_exists(icon)) {
        const char *base = strrchr(icon, '/');
        base = base ? base + 1 : icon;
        const char *dot = strrchr(base, '.');
        char icon_filename[300];
#pragma GCC diagnostic push
#pragma GCC diagnostic ignored "-Wformat-truncation"
        if (dot) snprintf(icon_filename, sizeof(icon_filename), "%s%s", slug, dot);
        else snprintf(icon_filename, sizeof(icon_filename), "%s.png", slug);

        char dest_icon[PATH_MAX];
        snprintf(dest_icon, sizeof(dest_icon), "%s/%s", icons_dir, icon_filename);
#pragma GCC diagnostic pop
        if (copy_file(icon, dest_icon, 0644) == 0) {
            snprintf(icon_value, sizeof(icon_value), "%s", dest_icon);
            d_step(t("ai_step_icon_copied"), dest_icon);
        } else {
            snprintf(icon_value, sizeof(icon_value), "%s", icon);
        }
    } else {
        /* Nombre de ícono del tema del sistema (ej. "firefox") */
        snprintf(icon_value, sizeof(icon_value), "%s", icon);
    }

    /* Crea el archivo .desktop */
    char desktop_path[PATH_MAX];
#pragma GCC diagnostic push
#pragma GCC diagnostic ignored "-Wformat-truncation"
    snprintf(desktop_path, sizeof(desktop_path), "%s/%s.desktop", desktop_dir, slug);
#pragma GCC diagnostic pop
    FILE *f = fopen(desktop_path, "w");
    if (!f) {
        d_err("%s", t("ai_err_desktop"));
        return;
    }
    fprintf(f,
        "[Desktop Entry]\n"
        "Type=Application\n"
        "Name=%s\n"
        "Comment=%s\n"
        "Exec=\"%s\" %%U\n"
        "Icon=%s\n"
        "Terminal=false\n"
        "Categories=%s\n"
        "X-FPM-AppImage=true\n",
        name, desc, dest_appimage, icon_value, category);
    fclose(f);
    chmod(desktop_path, 0644);
    d_step(t("ai_step_desktop_created"), desktop_path);

    /* Refresca la base de datos de escritorio si la herramienta existe */
    if (cmd_exists("update-desktop-database")) {
        char *argv[] = { "update-desktop-database", desktop_dir, NULL };
        run_cmd(argv, 1);
    }

    d_ok("%s", t("ai_ok_installed"));
    printf("  %s*%s %s: %s\n", C_BGREEN, C_RESET, t("lbl_name"), name);
    printf("  %s*%s %s: %s\n", C_BGREEN, C_RESET, t("lbl_binary"), dest_appimage);
    printf("  %s*%s %s: %s\n", C_BGREEN, C_RESET, t("lbl_launcher"), desktop_path);
    printf("  %s*%s %s: %s\n", C_BGREEN, C_RESET, t("lbl_size"), src_sizebuf);
}

static void local_install(const char *path) {
    if (!path || !file_exists(path)) {
        d_err("%s", t("ai_err_not_found"));
        return;
    }

    char guessed[256];
    guess_name_from_path(path, guessed, sizeof(guessed));

    d_info("%s", t("ai_info_questions"));

    char name[256], desc[512], icon[512], category[32];
    ask_input(t("ai_ask_name"), guessed, name, sizeof(name));
    ask_input(t("ai_ask_desc"), t("ai_default_desc"), desc, sizeof(desc));
    ask_category(category, sizeof(category));
    ask_input(t("ai_ask_icon"), "application-x-executable", icon, sizeof(icon));

    install_core(path, name, desc, category, icon);
}

/* Instalación no interactiva: usada por frontends gráficos (ej. yl-soft)
 * que ya recolectaron nombre/descripción/categoría/ícono por su cuenta. */
void fpm_appimage_install_args(const char *path, const char *name,
                                const char *desc, const char *category,
                                const char *icon) {
    if (!path || !file_exists(path)) {
        d_err("%s", t("ai_err_not_found"));
        return;
    }
    char guessed[256];
    if (!name || !*name) {
        guess_name_from_path(path, guessed, sizeof(guessed));
        name = guessed;
    }
    if (!desc || !*desc) desc = t("ai_default_desc");
    if (!category || !*category) category = "Utility";
    if (!icon || !*icon) icon = "application-x-executable";

    install_core(path, name, desc, category, icon);
}



static void local_remove(void) {
    char bin_dir[PATH_MAX], apps_dir[PATH_MAX], icons_dir[PATH_MAX], desktop_dir[PATH_MAX];
    get_dirs(bin_dir, sizeof(bin_dir), apps_dir, sizeof(apps_dir),
              icons_dir, sizeof(icons_dir), desktop_dir, sizeof(desktop_dir));

    InstalledApp apps[256];
    int count = scan_installed(desktop_dir, apps_dir, apps, 256);

    d_header(t("ai_hdr_remove"));
    if (count == 0) {
        d_info("%s", t("ai_info_none"));
        return;
    }

    int idx = select_installed(apps, count);
    if (idx < 0) return;

    int removed = 0;
    if (file_exists(apps[idx].appimage_path)) {
        remove(apps[idx].appimage_path);
        removed = 1;
        d_step(t("ai_step_removed_bin"), apps[idx].appimage_path);
    }
    if (file_exists(apps[idx].desktop_path)) {
        remove(apps[idx].desktop_path);
        removed = 1;
        d_step(t("ai_step_removed_desktop"), apps[idx].desktop_path);
    }

    if (!removed) d_warn("%s", t("ai_warn_not_installed"));
    else d_ok("%s", t("ai_ok_removed"));
}

static void local_update(const char *new_path) {
    if (!new_path || !file_exists(new_path)) {
        d_err("%s", t("ai_err_not_found"));
        return;
    }

    char bin_dir[PATH_MAX], apps_dir[PATH_MAX], icons_dir[PATH_MAX], desktop_dir[PATH_MAX];
    get_dirs(bin_dir, sizeof(bin_dir), apps_dir, sizeof(apps_dir),
              icons_dir, sizeof(icons_dir), desktop_dir, sizeof(desktop_dir));

    InstalledApp apps[256];
    int count = scan_installed(desktop_dir, apps_dir, apps, 256);

    d_header(t("ai_hdr_update"));
    if (count == 0) {
        d_info("%s", t("ai_info_none"));
        return;
    }

    int idx = select_installed(apps, count);
    if (idx < 0) return;

    if (copy_file(new_path, apps[idx].appimage_path, 0755) != 0) {
        d_err("%s", t("ai_err_update_failed"));
        return;
    }

    d_ok("%s", t("ai_ok_updated"));
    printf("  %s*%s %s: %s\n", C_BGREEN, C_RESET, t("lbl_name"), apps[idx].name);
    printf("  %s*%s %s: %s\n", C_BGREEN, C_RESET, t("lbl_binary"), apps[idx].appimage_path);
}

static int find_by_name(InstalledApp *apps, int count, const char *name) {
    for (int i = 0; i < count; i++)
        if (strcmp(apps[i].name, name) == 0) return i;
    return -1;
}

/* Elimina una app instalada identificada por nombre exacto (tal como
 * aparece en 'Name=' del .desktop). Usada por frontends gráficos que ya
 * conocen el nombre y no necesitan un menú interactivo. */
void fpm_appimage_remove_by_name(const char *name) {
    char bin_dir[PATH_MAX], apps_dir[PATH_MAX], icons_dir[PATH_MAX], desktop_dir[PATH_MAX];
    get_dirs(bin_dir, sizeof(bin_dir), apps_dir, sizeof(apps_dir),
              icons_dir, sizeof(icons_dir), desktop_dir, sizeof(desktop_dir));

    InstalledApp apps[256];
    int count = scan_installed(desktop_dir, apps_dir, apps, 256);
    int idx = find_by_name(apps, count, name);
    if (idx < 0) {
        d_err("%s", t("ai_info_none"));
        return;
    }

    int removed = 0;
    if (file_exists(apps[idx].appimage_path)) {
        remove(apps[idx].appimage_path);
        removed = 1;
        d_step(t("ai_step_removed_bin"), apps[idx].appimage_path);
    }
    if (file_exists(apps[idx].desktop_path)) {
        remove(apps[idx].desktop_path);
        removed = 1;
        d_step(t("ai_step_removed_desktop"), apps[idx].desktop_path);
    }

    if (!removed) d_warn("%s", t("ai_warn_not_installed"));
    else d_ok("%s", t("ai_ok_removed"));
}

/* Reemplaza el binario de la app 'name' con 'new_path'. Igual que
 * fpm_appimage_remove_by_name, pensado para uso no interactivo. */
void fpm_appimage_update_by_name(const char *name, const char *new_path) {
    if (!new_path || !file_exists(new_path)) {
        d_err("%s", t("ai_err_not_found"));
        return;
    }

    char bin_dir[PATH_MAX], apps_dir[PATH_MAX], icons_dir[PATH_MAX], desktop_dir[PATH_MAX];
    get_dirs(bin_dir, sizeof(bin_dir), apps_dir, sizeof(apps_dir),
              icons_dir, sizeof(icons_dir), desktop_dir, sizeof(desktop_dir));

    InstalledApp apps[256];
    int count = scan_installed(desktop_dir, apps_dir, apps, 256);
    int idx = find_by_name(apps, count, name);
    if (idx < 0) {
        d_err("%s", t("ai_info_none"));
        return;
    }

    if (copy_file(new_path, apps[idx].appimage_path, 0755) != 0) {
        d_err("%s", t("ai_err_update_failed"));
        return;
    }
    d_ok("%s", t("ai_ok_updated"));
}

/* Lista las apps instaladas en formato estable, una por línea, separado
 * por tabs: nombre \t bytes \t ruta_appimage \t ruta_desktop \t categoria
 * Pensado para ser parseado por frontends gráficos (ej. yl-soft). */
void fpm_appimage_list_plain(void) {
    char bin_dir[PATH_MAX], apps_dir[PATH_MAX], icons_dir[PATH_MAX], desktop_dir[PATH_MAX];
    get_dirs(bin_dir, sizeof(bin_dir), apps_dir, sizeof(apps_dir),
              icons_dir, sizeof(icons_dir), desktop_dir, sizeof(desktop_dir));

    InstalledApp apps[256];
    int count = scan_installed(desktop_dir, apps_dir, apps, 256);

    for (int i = 0; i < count; i++) {
        printf("%s\t%ld\t%s\t%s\t%s\n", apps[i].name, apps[i].size,
               apps[i].appimage_path, apps[i].desktop_path, apps[i].category);
    }
}


static int ask_confirm(const char *question) {
    printf("%s?%s %s [s/N] ", C_BYELLOW, C_RESET, question);
    fflush(stdout);
    char line[16];
    if (!fgets(line, sizeof(line), stdin)) return 0;
    return line[0] == 's' || line[0] == 'S' || line[0] == 'y' || line[0] == 'Y';
}

/* Menú de acciones sobre un AppImage ya seleccionado en la pantalla
 * curses: ver información, actualizarlo con otro archivo, o
 * eliminarlo. */
static void manage_actions(InstalledApp *app) {
    char sizebuf[32];
    if (app->size >= 0) format_size(app->size, sizebuf, sizeof(sizebuf));
    else snprintf(sizebuf, sizeof(sizebuf), "%s", t("info_size_unknown"));

    d_header(t("ai_hdr_manage_selected"));
    printf("  %s*%s %s: %s\n", C_BGREEN, C_RESET, t("lbl_name"), app->name);
    printf("  %s*%s %s: %s\n", C_BGREEN, C_RESET, t("lbl_binary"), app->appimage_path);
    printf("  %s*%s %s: %s\n", C_BGREEN, C_RESET, t("lbl_size"), sizebuf);
    if (app->category[0]) printf("  %s*%s %s: %s\n", C_BGREEN, C_RESET, t("lbl_category"), app->category);

    printf("\n%s%s?%s %s\n", C_BOLD, C_BYELLOW, C_RESET, t("ai_ask_action"));
    printf("    %s1)%s %s\n", C_BCYAN, C_RESET, t("ai_action_info"));
    printf("    %s2)%s %s\n", C_BCYAN, C_RESET, t("ai_action_update"));
    printf("    %s3)%s %s\n", C_BCYAN, C_RESET, t("ai_action_remove"));
    printf("    %s0)%s %s\n", C_BCYAN, C_RESET, t("ai_action_cancel"));
    printf("  %s>%s ", C_BYELLOW, C_RESET);
    fflush(stdout);

    char line[16];
    if (!fgets(line, sizeof(line), stdin)) return;
    int choice = atoi(line);

    switch (choice) {
        case 1:
            d_ok("%s", t("ai_ok_info_shown"));
            break;
        case 2: {
            char new_path[PATH_MAX];
            ask_input(t("ai_ask_new_file"), NULL, new_path, sizeof(new_path));
            if (!new_path[0] || !file_exists(new_path)) {
                d_err("%s", t("ai_err_not_found"));
                break;
            }
            if (copy_file(new_path, app->appimage_path, 0755) != 0) {
                d_err("%s", t("ai_err_update_failed"));
            } else {
                d_ok("%s", t("ai_ok_updated"));
            }
            break;
        }
        case 3:
            if (ask_confirm(t("ai_ask_confirm_remove"))) {
                int removed = 0;
                if (file_exists(app->appimage_path)) {
                    remove(app->appimage_path);
                    removed = 1;
                    d_step(t("ai_step_removed_bin"), app->appimage_path);
                }
                if (file_exists(app->desktop_path)) {
                    remove(app->desktop_path);
                    removed = 1;
                    d_step(t("ai_step_removed_desktop"), app->desktop_path);
                }
                if (removed) d_ok("%s", t("ai_ok_removed"));
                else d_warn("%s", t("ai_warn_not_installed"));
            } else {
                d_info("%s", t("info_remove_cancelled"));
            }
            break;
        default:
            d_info("%s", t("info_none_selected"));
            break;
    }
}

static void local_manage(void) {
    char bin_dir[PATH_MAX], apps_dir[PATH_MAX], icons_dir[PATH_MAX], desktop_dir[PATH_MAX];
    get_dirs(bin_dir, sizeof(bin_dir), apps_dir, sizeof(apps_dir),
              icons_dir, sizeof(icons_dir), desktop_dir, sizeof(desktop_dir));

    InstalledApp apps[256];
    int count = scan_installed(desktop_dir, apps_dir, apps, 256);

    d_header(t("ai_hdr_manage"));
    if (count == 0) {
        d_info("%s", t("ai_info_none"));
        return;
    }

    if (!is_tty()) {
        d_err("%s", t("err_select_needs_tty"));
        return;
    }

    /* Prepara los ítems para la interfaz curses: nombre + tamaño como
     * descripción. */
    TuiItem items[256];
    char sizebufs[256][32];
    for (int i = 0; i < count; i++) {
        if (apps[i].size >= 0) format_size(apps[i].size, sizebufs[i], sizeof(sizebufs[i]));
        else snprintf(sizebufs[i], sizeof(sizebufs[i]), "%s", t("info_size_unknown"));
        items[i].name = apps[i].name;
        items[i].desc = sizebufs[i];
    }

    int sel_count = 0;
    char **selected = tui_select(items, count, "FPM - AppImages", &sel_count);
    if (!selected || sel_count == 0) {
        d_info("%s", t("info_none_selected"));
        free(selected);
        return;
    }

    /* Solo se gestiona una aplicación a la vez; se usa la primera
     * marcada si el usuario seleccionó más de una. */
    int idx = -1;
    for (int i = 0; i < count; i++) {
        if (strcmp(selected[0], apps[i].name) == 0) { idx = i; break; }
    }
    free(selected);

    if (idx < 0) return;
    manage_actions(&apps[idx]);
}

static void local_list(void) {
    char bin_dir[PATH_MAX], apps_dir[PATH_MAX], icons_dir[PATH_MAX], desktop_dir[PATH_MAX];
    get_dirs(bin_dir, sizeof(bin_dir), apps_dir, sizeof(apps_dir),
              icons_dir, sizeof(icons_dir), desktop_dir, sizeof(desktop_dir));

    d_header(t("ai_hdr_list"));

    InstalledApp apps[256];
    int count = scan_installed(desktop_dir, apps_dir, apps, 256);

    for (int i = 0; i < count; i++) {
        char sizebuf[32];
        if (apps[i].size >= 0) format_size(apps[i].size, sizebuf, sizeof(sizebuf));
        else snprintf(sizebuf, sizeof(sizebuf), "%s", t("info_size_unknown"));
        printf("  %s*%s %s %s(%s)%s\n", C_BGREEN, C_RESET, apps[i].name, C_DIM, sizebuf, C_RESET);
    }

    if (count == 0) d_info("%s", t("ai_info_none"));
}

/* --- Backend local (por defecto): implementación basada en .desktop --- */
static const AppImageBackend local_backend = {
    .name = "local",
    .install = local_install,
    .remove_app = local_remove,
    .update = local_update,
    .list = local_list,
    .manage = local_manage,
};

static const AppImageBackend *g_backend = &local_backend;

void fpm_appimage_set_backend(const AppImageBackend *backend) {
    g_backend = backend ? backend : &local_backend;
}

const AppImageBackend *fpm_appimage_get_backend(void) {
    return g_backend;
}

/* --- API pública: despacha al backend activo --- */
void fpm_appimage_install(const char *path) { g_backend->install(path); }
void fpm_appimage_remove(void) { g_backend->remove_app(); }
void fpm_appimage_update(const char *new_path) { g_backend->update(new_path); }
void fpm_appimage_list(void) { g_backend->list(); }
void fpm_appimage_manage(void) { g_backend->manage(); }
