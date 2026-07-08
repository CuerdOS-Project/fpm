/* SPDX-License-Identifier: GPL-3.0-or-later */
#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <limits.h>
#include <sys/wait.h>
#include "utils.h"
#include "display.h"
#include "i18n.h"

int g_verbose = 0;
int g_yes = 0;

static const char *REQUIRES_ROOT[] = {
    "install", "remove", "superremove", "update", "upgrade", "repair",
    "clean", "check", "hold", "unhold", "autoremove", NULL
};

int cmd_exists(const char *name) {
    const char *path = getenv("PATH");
    if (!path) return 0;
    char *copy = strdup(path);
    if (!copy) return 0;
    char *saveptr = NULL;
    char *dir = strtok_r(copy, ":", &saveptr);
    int found = 0;
    while (dir) {
        char full[PATH_MAX];
        snprintf(full, sizeof(full), "%s/%s", dir, name);
        if (access(full, X_OK) == 0) { found = 1; break; }
        dir = strtok_r(NULL, ":", &saveptr);
    }
    free(copy);
    return found;
}

static int command_requires_root(const char *command) {
    for (int i = 0; REQUIRES_ROOT[i]; i++) {
        if (strcmp(REQUIRES_ROOT[i], command) == 0) return 1;
    }
    return 0;
}

void elevate_if_needed(const char *command, int argc, char *argv[]) {
    if (geteuid() == 0 || !command_requires_root(command)) return;

    const char *elevator = NULL;
    if (cmd_exists("doas")) elevator = "doas";
    else if (cmd_exists("sudo")) elevator = "sudo";

    if (!elevator) {
        d_err("%s", t("err_no_privilege_tool"));
        exit(1);
    }

    d_info(t("info_elevating"), command, elevator);

    char self_path[PATH_MAX];
    ssize_t len = readlink("/proc/self/exe", self_path, sizeof(self_path) - 1);
    const char *exe = argv[0];
    if (len > 0) {
        self_path[len] = '\0';
        exe = self_path;
    }

    char **new_argv = malloc(sizeof(char *) * (argc + 2));
    new_argv[0] = (char *)elevator;
    new_argv[1] = (char *)exe;
    for (int i = 1; i < argc; i++) new_argv[i + 1] = argv[i];
    new_argv[argc + 1] = NULL;

    execvp(elevator, new_argv);
    /* Si execvp regresa, algo falló */
    d_err("%s", t("err_elevation_failed"));
    exit(1);
}

RunResult run_cmd(char *const argv[], int capture) {
    RunResult result = { .exit_code = -1, .stdout_data = NULL };

    if (g_verbose && !capture) {
        printf("%s$", C_DIM);
        for (int i = 0; argv[i]; i++) printf(" %s", argv[i]);
        printf("%s\n", C_RESET);
    }

    int pipefd[2];
    if (capture) {
        if (pipe(pipefd) != 0) return result;
    }

    pid_t pid = fork();
    if (pid < 0) {
        if (capture) { close(pipefd[0]); close(pipefd[1]); }
        return result;
    }

    if (pid == 0) {
        if (capture) {
            dup2(pipefd[1], STDOUT_FILENO);
            close(pipefd[0]);
            close(pipefd[1]);
        }
        execvp(argv[0], argv);
        _exit(127);
    }

    char *buffer = NULL;
    size_t total = 0;
    if (capture) {
        close(pipefd[1]);
        buffer = malloc(FPM_MAX_OUTPUT);
        buffer[0] = '\0';
        ssize_t n;
        char chunk[4096];
        while ((n = read(pipefd[0], chunk, sizeof(chunk))) > 0) {
            if (total + (size_t)n < FPM_MAX_OUTPUT - 1) {
                memcpy(buffer + total, chunk, n);
                total += n;
                buffer[total] = '\0';
            }
        }
        close(pipefd[0]);
    }

    int status = 0;
    waitpid(pid, &status, 0);
    result.exit_code = WIFEXITED(status) ? WEXITSTATUS(status) : -1;
    result.stdout_data = buffer;
    return result;
}
