/* SPDX-License-Identifier: GPL-3.0-or-later */
#define _GNU_SOURCE
#include <ncurses.h>
#include <stdlib.h>
#include <string.h>
#include <strings.h>
#include "tui.h"
#include "i18n.h"

typedef struct {
    TuiItem *all;
    int all_count;
    int *filtered;      /* índices dentro de 'all' */
    int filtered_count;
    int *selected;       /* arreglo paralelo a 'all', 1 si está seleccionado */
    int cursor;
    int offset;
    char query[256];
} TuiState;

static void apply_filter(TuiState *st) {
    st->filtered_count = 0;
    if (st->query[0] == '\0') {
        for (int i = 0; i < st->all_count; i++) st->filtered[st->filtered_count++] = i;
    } else {
        for (int i = 0; i < st->all_count; i++) {
            int match = strcasestr(st->all[i].name, st->query) != NULL;
            if (!match && st->all[i].desc) {
                match = strcasestr(st->all[i].desc, st->query) != NULL;
            }
            if (match) st->filtered[st->filtered_count++] = i;
        }
    }
    st->cursor = 0;
    st->offset = 0;
}

static char **collect_selection(TuiState *st, int *out_count) {
    int n = 0;
    for (int i = 0; i < st->all_count; i++) if (st->selected[i]) n++;
    if (n == 0) { *out_count = 0; return NULL; }
    char **result = malloc(sizeof(char *) * n);
    int k = 0;
    for (int i = 0; i < st->all_count; i++) {
        if (st->selected[i]) result[k++] = st->all[i].name;
    }
    *out_count = n;
    return result;
}

char **tui_select(TuiItem *items, int count, const char *title, int *out_count) {
    *out_count = 0;
    if (count == 0) return NULL;

    TuiState st;
    st.all = items;
    st.all_count = count;
    st.filtered = malloc(sizeof(int) * count);
    st.selected = calloc(count, sizeof(int));
    st.query[0] = '\0';
    apply_filter(&st);

    initscr();
    cbreak();
    noecho();
    keypad(stdscr, TRUE);
    curs_set(0);
    start_color();
    use_default_colors();
    init_pair(1, COLOR_BLACK, COLOR_CYAN);
    init_pair(2, COLOR_WHITE, COLOR_MAGENTA);
    init_pair(3, COLOR_WHITE, COLOR_BLUE);
    init_pair(4, COLOR_YELLOW, -1);
    init_pair(5, COLOR_CYAN, -1);
    init_pair(6, COLOR_BLACK, COLOR_YELLOW);
    mousemask(BUTTON1_CLICKED | BUTTON1_DOUBLE_CLICKED, NULL);

    int quit = 0, confirmed = 0;
    while (!quit) {
        erase();
        int h, w;
        getmaxyx(stdscr, h, w);

        attron(COLOR_PAIR(5) | A_BOLD);
        mvhline(0, 0, ' ', w);
        mvprintw(0, 0, " %s ", title);
        attroff(COLOR_PAIR(5) | A_BOLD);

        attron(A_BOLD);
        mvprintw(1, 0, " %s: ", t("tui_filter_label"));
        attroff(A_BOLD);
        int label_len = (int)strlen(t("tui_filter_label")) + 3;
        attron(COLOR_PAIR(4));
        mvprintw(1, label_len, "%s%s", st.query, "\u2588");
        attroff(COLOR_PAIR(4));

        int selected_count = 0;
        for (int i = 0; i < st.all_count; i++) if (st.selected[i]) selected_count++;
        mvprintw(2, 0, " %d %s | %d %s ",
                 st.filtered_count, t("tui_results"),
                 selected_count, t("tui_selected"));

        int list_rows = h - 6 > 0 ? h - 6 : 1;
        for (int i = 0; i < list_rows && i + st.offset < st.filtered_count; i++) {
            int idx = st.filtered[i + st.offset];
            int row = 4 + i;
            int is_cursor = (i + st.offset) == st.cursor;
            int is_selected = st.selected[idx];
            int attr = A_NORMAL;
            if (is_cursor && is_selected) attr = COLOR_PAIR(3) | A_BOLD;
            else if (is_cursor) attr = COLOR_PAIR(1) | A_BOLD;
            else if (is_selected) attr = COLOR_PAIR(2) | A_BOLD;
            attron(attr);
            mvprintw(row, 0, "%s %-20s %s", is_selected ? "[x]" : "[ ]",
                     st.all[idx].name, st.all[idx].desc ? st.all[idx].desc : "");
            attroff(attr);
        }

        attron(COLOR_PAIR(6));
        mvhline(h - 1, 0, ' ', w);
        mvprintw(h - 1, 0, " %s ", t("tui_help"));
        attroff(COLOR_PAIR(6));

        refresh();
        int key = getch();

        if (key == 'q' || key == 'Q' || key == 27) { quit = 1; break; }
        if (key == '\n' || key == KEY_ENTER) { quit = 1; confirmed = 1; break; }
        if ((key == KEY_UP || key == 'k') && st.cursor > 0) {
            st.cursor--;
            if (st.cursor < st.offset) st.offset = st.cursor;
        } else if ((key == KEY_DOWN || key == 'j') && st.cursor < st.filtered_count - 1) {
            st.cursor++;
            if (st.cursor >= st.offset + list_rows) st.offset = st.cursor - list_rows + 1;
        } else if (key == ' ' && st.filtered_count > 0) {
            int idx = st.filtered[st.cursor];
            st.selected[idx] = !st.selected[idx];
        } else if ((key == 'a' || key == 'A') && st.filtered_count > 0) {
            int all_selected = 1;
            for (int i = 0; i < st.filtered_count; i++)
                if (!st.selected[st.filtered[i]]) { all_selected = 0; break; }
            for (int i = 0; i < st.filtered_count; i++)
                st.selected[st.filtered[i]] = !all_selected;
        } else if (key == KEY_BACKSPACE || key == 127 || key == 8) {
            size_t len = strlen(st.query);
            if (len > 0) st.query[len - 1] = '\0';
            apply_filter(&st);
        } else if (key >= 32 && key < 127) {
            size_t len = strlen(st.query);
            if (len < sizeof(st.query) - 1) {
                st.query[len] = (char)key;
                st.query[len + 1] = '\0';
                apply_filter(&st);
            }
        } else if (key == KEY_MOUSE) {
            MEVENT event;
            if (getmouse(&event) == OK) {
                if (event.y >= 4 && event.y < 4 + list_rows) {
                    int idx_in_view = event.y - 4 + st.offset;
                    if (idx_in_view < st.filtered_count) {
                        st.cursor = idx_in_view;
                        int idx = st.filtered[idx_in_view];
                        if (event.bstate & BUTTON1_DOUBLE_CLICKED) {
                            st.selected[idx] = 1;
                            quit = 1; confirmed = 1;
                        } else if (event.bstate & BUTTON1_CLICKED) {
                            st.selected[idx] = !st.selected[idx];
                        }
                    }
                }
            }
        }
    }

    endwin();

    char **result = NULL;
    if (confirmed) {
        result = collect_selection(&st, out_count);
    }
    free(st.filtered);
    free(st.selected);
    return result;
}
