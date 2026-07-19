#ifndef FPM_FLATPAK_H
#define FPM_FLATPAK_H

/* Verifica que el comando 'flatpak' esté disponible. */
int fpm_flatpak_available(void);

void fpm_flatpak_install(char **packages, int count);
void fpm_flatpak_remove(char **packages, int count);
void fpm_flatpak_update(char **packages, int count); /* count == 0 -> actualiza todo */
void fpm_flatpak_search(const char *query);
void fpm_flatpak_list(void);
void fpm_flatpak_info(const char *package);

#endif
