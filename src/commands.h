#ifndef FPM_COMMANDS_H
#define FPM_COMMANDS_H

/* Verifica que xbps esté disponible. Termina el programa si no lo está. */
void fpm_check_available(void);

void fpm_install(char **packages, int count);
void fpm_remove(char **packages, int count);
void fpm_superremove(char **packages, int count);
void fpm_update(void);
void fpm_upgrade(void);
void fpm_search(const char *query);
void fpm_info(const char *package);
void fpm_list(int show_all, int select);
void fpm_repair(void);
void fpm_clean(void);
void fpm_check(void);
void fpm_files(const char *package);
void fpm_owns(const char *path);
void fpm_deps(const char *package, int reverse);
void fpm_hold(char **packages, int count, int unhold);

/* Nuevas funcionalidades */
void fpm_orphans(int remove);          /* 'orphans' y 'autoremove' */
void fpm_size(char **packages, int count); /* tamaño instalado de paquete(s) */
void fpm_diskusage(void);              /* uso de disco de la caché de xbps */

#endif
