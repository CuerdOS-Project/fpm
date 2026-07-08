#ifndef FPM_DISPLAY_H
#define FPM_DISPLAY_H

#define C_RESET   "\033[0m"
#define C_BOLD    "\033[1m"
#define C_DIM     "\033[2m"
#define C_RED     "\033[31m"
#define C_GREEN   "\033[32m"
#define C_YELLOW  "\033[33m"
#define C_BLUE    "\033[34m"
#define C_MAGENTA "\033[35m"
#define C_CYAN    "\033[36m"
#define C_BRED    "\033[91m"
#define C_BGREEN  "\033[92m"
#define C_BYELLOW "\033[93m"
#define C_BBLUE   "\033[94m"
#define C_BMAGENTA "\033[95m"
#define C_BCYAN   "\033[96m"

void d_ok(const char *fmt, ...);
void d_err(const char *fmt, ...);
void d_warn(const char *fmt, ...);
void d_info(const char *fmt, ...);
void d_step(const char *fmt, ...);
void d_header(const char *title);

int is_tty(void);
void format_size(long bytes, char *out, size_t out_size);

#endif
