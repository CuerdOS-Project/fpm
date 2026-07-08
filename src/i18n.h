#ifndef FPM_I18N_H
#define FPM_I18N_H

/* Carga el catálogo de idioma apropiado detectando automáticamente el
 * idioma del sistema (LANGUAGE, LC_ALL, LC_MESSAGES, LANG). Busca los
 * archivos .kn en, en orden: $FPM_LOCALE_DIR, ./locales,
 * /usr/share/fpm/locales, /usr/local/share/fpm/locales.
 * Si no encuentra el idioma detectado, usa "en" como respaldo. */
void i18n_init(void);

/* Devuelve la cadena traducida asociada a 'key'. Si no existe, devuelve
 * 'key' tal cual, para que el programa nunca se quede sin texto. */
const char *t(const char *key);

/* Libera la memoria usada por el catálogo cargado. */
void i18n_shutdown(void);

/* Código del idioma actualmente activo (ej. "es", "en", "pt"). */
const char *i18n_current_lang(void);

#endif
