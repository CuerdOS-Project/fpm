/* SPDX-License-Identifier: GPL-3.0-or-later */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "i18n.h"

#define MAX_ENTRIES 512
#define MAX_LINE    1024
#define MAX_LANG    16

typedef struct {
    char *key;
    char *value;
} Entry;

static Entry entries[MAX_ENTRIES];
static int   entry_count = 0;
static char  current_lang[MAX_LANG] = "en";

static char *xstrdup(const char *s) {
    char *out = malloc(strlen(s) + 1);
    if (out) strcpy(out, s);
    return out;
}

/* Extrae el código de idioma de una cadena de locale tipo "es_MX.UTF-8"
 * o de una lista separada por ':' como en LANGUAGE. */
static void extract_lang_code(const char *raw, char *out, size_t out_size) {
    if (!raw || !*raw || strcmp(raw, "C") == 0 || strcmp(raw, "POSIX") == 0) {
        snprintf(out, out_size, "en");
        return;
    }
    size_t i = 0;
    while (raw[i] && raw[i] != '_' && raw[i] != '.' && raw[i] != ':' &&
           i < out_size - 1) {
        out[i] = raw[i];
        i++;
    }
    out[i] = '\0';
    if (i == 0) snprintf(out, out_size, "en");
}

static void detect_language(char *out, size_t out_size) {
    const char *vars[] = { "FPM_LANG", "LANGUAGE", "LC_ALL", "LC_MESSAGES", "LANG" };
    for (size_t i = 0; i < sizeof(vars) / sizeof(vars[0]); i++) {
        const char *val = getenv(vars[i]);
        if (val && *val) {
            /* LANGUAGE puede traer varias opciones separadas por ':' */
            char first[MAX_LANG * 4];
            snprintf(first, sizeof(first), "%s", val);
            char *colon = strchr(first, ':');
            if (colon) *colon = '\0';
            extract_lang_code(first, out, out_size);
            return;
        }
    }
    snprintf(out, out_size, "en");
}

static void trim(char *s) {
    char *start = s;
    while (*start == ' ' || *start == '\t') start++;
    if (start != s) memmove(s, start, strlen(start) + 1);
    size_t len = strlen(s);
    while (len > 0 && (s[len-1] == '\n' || s[len-1] == '\r' ||
                        s[len-1] == ' ' || s[len-1] == '\t')) {
        s[len-1] = '\0';
        len--;
    }
}

static int load_catalog_file(const char *path) {
    FILE *f = fopen(path, "r");
    if (!f) return 0;

    char line[MAX_LINE];
    entry_count = 0;
    while (fgets(line, sizeof(line), f) && entry_count < MAX_ENTRIES) {
        char *nl = strchr(line, '\n');
        if (nl) *nl = '\0';
        char *trimmed = line;
        while (*trimmed == ' ' || *trimmed == '\t') trimmed++;
        if (*trimmed == '\0' || *trimmed == '#') continue;

        char *eq = strchr(trimmed, '=');
        if (!eq) continue;
        *eq = '\0';
        char *key = trimmed;
        char *value = eq + 1;
        trim(key);
        trim(value);
        if (*key == '\0') continue;

        entries[entry_count].key   = xstrdup(key);
        entries[entry_count].value = xstrdup(value);
        entry_count++;
    }
    fclose(f);
    return 1;
}

static int try_load(const char *dir, const char *lang) {
    char path[512];
    snprintf(path, sizeof(path), "%s/%s.kn", dir, lang);
    return load_catalog_file(path);
}

void i18n_init(void) {
    char detected[MAX_LANG];
    detect_language(detected, sizeof(detected));

    const char *dirs[] = {
        getenv("FPM_LOCALE_DIR"),
        "./locales",
        "/usr/share/fpm/locales",
        "/usr/local/share/fpm/locales",
    };

    int loaded = 0;
    for (size_t i = 0; i < sizeof(dirs) / sizeof(dirs[0]) && !loaded; i++) {
        if (!dirs[i]) continue;
        if (try_load(dirs[i], detected)) {
            snprintf(current_lang, sizeof(current_lang), "%s", detected);
            loaded = 1;
        }
    }

    if (!loaded) {
        for (size_t i = 0; i < sizeof(dirs) / sizeof(dirs[0]) && !loaded; i++) {
            if (!dirs[i]) continue;
            if (try_load(dirs[i], "en")) {
                snprintf(current_lang, sizeof(current_lang), "en");
                loaded = 1;
            }
        }
    }
    /* Si no se encontró ningún catálogo, t() seguirá devolviendo las
     * claves tal cual, así que el programa sigue siendo funcional. */
}

const char *t(const char *key) {
    for (int i = 0; i < entry_count; i++) {
        if (strcmp(entries[i].key, key) == 0) {
            return entries[i].value;
        }
    }
    return key;
}

const char *i18n_current_lang(void) {
    return current_lang;
}

void i18n_shutdown(void) {
    for (int i = 0; i < entry_count; i++) {
        free(entries[i].key);
        free(entries[i].value);
    }
    entry_count = 0;
}
