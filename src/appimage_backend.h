/* SPDX-License-Identifier: GPL-3.0-or-later */
#ifndef FPM_APPIMAGE_BACKEND_H
#define FPM_APPIMAGE_BACKEND_H

/* Contrato que debe cumplir cualquier backend de gestión de AppImages.
 * FPM incluye un backend "local" por defecto (basado en archivos
 * .desktop, ver appimage.c). Un backend externo (ej. la app gráfica)
 * puede registrarse con fpm_appimage_set_backend() para reemplazar
 * install/remove/update/list/manage sin tocar el resto del programa. */
typedef struct {
    const char *name; /* identificador corto, ej. "local", "gui" */

    /* Instala 'path' de forma interactiva o automática según el backend. */
    void (*install)(const char *path);

    /* Elige (según el backend) y elimina una app instalada. */
    void (*remove_app)(void);

    /* Reemplaza el binario de una app instalada por 'new_path'. */
    void (*update)(const char *new_path);

    /* Lista las apps gestionadas por el backend. */
    void (*list)(void);

    /* Interfaz interactiva de gestión (ver info/actualizar/eliminar). */
    void (*manage)(void);
} AppImageBackend;

/* Registra 'backend' como backend activo. Pasar NULL restaura el
 * backend local por defecto. Los punteros de función no usados en
 * 'backend' deben apuntar a una implementación válida (no se permite
 * NULL): FPM no verifica esto y llamará al puntero directamente. */
void fpm_appimage_set_backend(const AppImageBackend *backend);

/* Backend actualmente activo (para diagnóstico o reuso parcial). */
const AppImageBackend *fpm_appimage_get_backend(void);

#endif
