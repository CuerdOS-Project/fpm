#ifndef FPM_TUI_H
#define FPM_TUI_H

typedef struct {
    char *name;
    char *desc; /* puede ser NULL */
} TuiItem;

/* Muestra una lista interactiva de paquetes con búsqueda, selección
 * múltiple (teclado y mouse) y devuelve los nombres seleccionados.
 * 'out_count' recibe el número de elementos seleccionados.
 * El llamador debe liberar el arreglo devuelto (no las cadenas, que
 * apuntan dentro de 'items'). Devuelve NULL si no se seleccionó nada. */
char **tui_select(TuiItem *items, int count, const char *title, int *out_count);

#endif
