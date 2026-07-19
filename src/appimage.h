#ifndef FPM_APPIMAGE_H
#define FPM_APPIMAGE_H

#include "appimage_backend.h"

/* Instala un AppImage de forma interactiva: pregunta nombre, descripción,
 * categoría (elegida por número) e ícono, copia el binario a
 * ~/.local/bin (o /opt si es root) y genera un archivo .desktop en
 * ~/.local/share/applications. */
void fpm_appimage_install(const char *path);

/* Muestra un menú numerado con los AppImages instalados por FPM y
 * elimina el que el usuario seleccione (binario + .desktop). */
void fpm_appimage_remove(void);

/* Muestra un menú numerado con los AppImages instalados por FPM y
 * reemplaza el binario del seleccionado con 'new_path' (mantiene el
 * mismo .desktop, nombre, ícono y categoría). */
void fpm_appimage_update(const char *new_path);

/* Lista los AppImages instalados por FPM (según sus .desktop). */
void fpm_appimage_list(void);

/* Abre una interfaz de texto con ncurses (curses) para elegir un
 * AppImage instalado y luego ver su información, actualizarlo o
 * eliminarlo desde un menú de acciones. */
void fpm_appimage_manage(void);

/* Instala sin diálogos interactivos: para frontends gráficos que ya
 * obtuvieron nombre/descripción/categoría/ícono por su cuenta. Cualquier
 * campo NULL o vacío se completa con un valor por defecto razonable. */
void fpm_appimage_install_args(const char *path, const char *name,
                                const char *desc, const char *category,
                                const char *icon);

/* Elimina/actualiza por nombre exacto, sin menú interactivo. */
void fpm_appimage_remove_by_name(const char *name);
void fpm_appimage_update_by_name(const char *name, const char *new_path);

/* Lista en formato estable "nombre\tbytes\truta_appimage\truta_desktop\tcategoria",
 * una app por línea, fácil de parsear desde otros programas. */
void fpm_appimage_list_plain(void);

#endif

