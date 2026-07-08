/* SPDX-License-Identifier: GPL-3.0-or-later */
#define _POSIX_C_SOURCE 200809L
#include <stdio.h>
#include <stdarg.h>
#include <unistd.h>
#include "display.h"

static void vprint(FILE *out, const char *icon_color, const char *icon,
                    const char *fmt, va_list args) {
    fprintf(out, "%s%s%s ", icon_color, icon, C_RESET);
    vfprintf(out, fmt, args);
    fprintf(out, "\n");
}

void d_ok(const char *fmt, ...) {
    va_list args; va_start(args, fmt);
    vprint(stdout, C_BGREEN, "\u2713", fmt, args);
    va_end(args);
}

void d_err(const char *fmt, ...) {
    va_list args; va_start(args, fmt);
    vprint(stderr, C_BRED, "\u2717", fmt, args);
    va_end(args);
}

void d_warn(const char *fmt, ...) {
    va_list args; va_start(args, fmt);
    vprint(stdout, C_BYELLOW, "\u26a0", fmt, args);
    va_end(args);
}

void d_info(const char *fmt, ...) {
    va_list args; va_start(args, fmt);
    vprint(stdout, C_BBLUE, "\u2139", fmt, args);
    va_end(args);
}

void d_step(const char *fmt, ...) {
    va_list args; va_start(args, fmt);
    vprint(stdout, C_BMAGENTA, "\u2192", fmt, args);
    va_end(args);
}

void d_header(const char *title) {
    printf("\n%s%s%s%s\n", C_BOLD, C_BCYAN, title, C_RESET);
    for (int i = 0; i < 40; i++) putchar(0xE2), putchar(0x94), putchar(0x80);
    putchar('\n');
}

int is_tty(void) {
    return isatty(fileno(stdout)) && isatty(fileno(stdin));
}

void format_size(long bytes, char *out, size_t out_size) {
    const char *units[] = { "B", "KB", "MB", "GB", "TB" };
    double n = (double)bytes;
    int u = 0;
    while (n >= 1024.0 && u < 4) {
        n /= 1024.0;
        u++;
    }
    if (u == 0) {
        snprintf(out, out_size, "%ld %s", bytes, units[u]);
    } else {
        snprintf(out, out_size, "%.1f %s", n, units[u]);
    }
}
