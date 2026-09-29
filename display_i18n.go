package main

import (
	"bufio"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	reset  = "\033[0m"
	bold   = "\033[1m"
	dim    = "\033[2m"
	green  = "\033[1;32m"
	yellow = "\033[1;33m"
	cyan   = "\033[1;36m"
	red    = "\033[1;31m"
)

func isTTY() bool {
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func dOK(format string, args ...any) {
	fmt.Printf("%s✓%s %s\n", green, reset, fmt.Sprintf(format, args...))
}
func dErr(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "%s✗%s %s\n", red, reset, fmt.Sprintf(format, args...))
}
func dWarn(format string, args ...any) {
	fmt.Printf("%s!%s %s\n", yellow, reset, fmt.Sprintf(format, args...))
}
func dInfo(format string, args ...any) {
	fmt.Printf("%s•%s %s\n", cyan, reset, fmt.Sprintf(format, args...))
}
func dStep(format string, args ...any) {
	fmt.Printf("  %s→%s %s\n", cyan, reset, fmt.Sprintf(format, args...))
}
func dHeader(format string, args ...any) {
	fmt.Printf("\n%s%s%s%s\n", bold, cyan, fmt.Sprintf(format, args...), reset)
}

func formatSize(n int64) string {
	if n < 0 {
		return tr("info_size_unknown")
	}
	units := []string{"B", "K", "M", "G", "T"}
	value := float64(n)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d %s", n, units[unit])
	}
	return fmt.Sprintf("%.1f %s", value, units[unit])
}

// Los catálogos se compilan dentro del binario para que una instalación no
// dependa de una ruta relativa al directorio de ejecución.
//
//go:embed locales/*.kn
var localeFS embed.FS

var catalog map[string]string

func languageCode() string {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		value := os.Getenv(key)
		if value == "" {
			continue
		}
		value = strings.ToLower(strings.Split(strings.Split(value, ".")[0], "@")[0])
		if len(value) >= 2 {
			return value[:2]
		}
	}
	return "en"
}

func initI18n() {
	catalog = make(map[string]string)
	lang := languageCode()
	data, err := fs.ReadFile(localeFS, filepath.Join("locales", lang+".kn"))
	if err != nil {
		data, _ = fs.ReadFile(localeFS, "locales/en.kn")
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if ok {
			catalog[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
}

func tr(key string) string {
	if value, ok := catalog[key]; ok {
		return value
	}
	return key
}
