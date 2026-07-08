#ifndef FPM_UTILS_H
#define FPM_UTILS_H

#define FPM_MAX_OUTPUT (1 << 20) /* 1 MB */

typedef struct {
    int   exit_code;
    char *stdout_data; /* debe liberarse con free() si no es NULL */
} RunResult;

extern int g_verbose;
extern int g_yes; /* bandera -y/--yes: responde "sí" a las confirmaciones */

/* Ejecuta 'argv' (terminado en NULL). Si 'capture' es distinto de cero,
 * captura stdout en el resultado; de lo contrario lo hereda del padre. */
RunResult run_cmd(char *const argv[], int capture);

/* Verifica si un ejecutable existe en el PATH. */
int cmd_exists(const char *name);

/* Reejecuta el proceso actual con doas/sudo si el comando lo requiere
 * y el usuario actual no es root. */
void elevate_if_needed(const char *command, int argc, char *argv[]);

#endif
