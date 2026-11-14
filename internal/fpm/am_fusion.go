package fpm

import (
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	amFusionRepoName = "am-FUSION"
	amFusionRepoURL  = "https://github.com/ivan-hc/AM"
)

// amFusionDBPathOverride is set only by tests. Production stores one FPM
// AppImage database for both FPM-local and am-FUSION in the user's XDG data dir.
var amFusionDBPathOverride string

var amFusionNamePattern = regexp.MustCompile(`^[[:alnum:]][[:alnum:]_.+-]*$`)
var amFusionANSI = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
var amFusionSizePattern = regexp.MustCompile(`(?i)^([0-9]+(?:\.[0-9]+)?)\s*([a-z]+)$`)

type amFusionPackage struct {
	Name        string
	Description string
	Version     string
	Type        string
	SizeBytes   int64
	Installed   bool
	UpdatedAt   string
}

func currentAMFusionDBPath() string {
	if amFusionDBPathOverride != "" {
		return amFusionDBPathOverride
	}
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" || !filepath.IsAbs(dataHome) {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			home = os.Getenv("HOME")
		}
		if home == "" {
			home = "/tmp"
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataHome, "fpm", "am-fusion.db")
}

func amFusionDatabaseExists() bool {
	_, err := os.Stat(currentAMFusionDBPath())
	return err == nil
}

func configuredAMBackend() string {
	if !amFusionDatabaseExists() {
		return ""
	}
	db, err := openAMFusionDB()
	if err != nil {
		return ""
	}
	defer db.Close()
	var backend string
	if err := db.QueryRow(`SELECT value FROM metadata WHERE key='am_backend'`).Scan(&backend); err != nil {
		return ""
	}
	if backend == "am" || backend == "appman" {
		return backend
	}
	return ""
}

func setAMFusionBackend(backend string) error {
	if backend != "am" && backend != "appman" {
		return fmt.Errorf("backend AM desconocido: %q", backend)
	}
	db, err := openAMFusionDB()
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO metadata(key, value) VALUES('am_backend', ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, backend)
	return err
}

func openAMFusionDB() (*sql.DB, error) {
	path := currentAMFusionDBPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("crear directorio de la base am-FUSION: %w", err)
	}
	if err := prepareFPMDBPath(path, os.Geteuid()); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=DELETE; PRAGMA synchronous=FULL; PRAGMA busy_timeout=5000;
		CREATE TABLE IF NOT EXISTS repositories (
			name TEXT PRIMARY KEY NOT NULL,
			url TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 0,
			last_sync TEXT NOT NULL DEFAULT ''
		);
		CREATE TABLE IF NOT EXISTS appimage_packages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			version TEXT NOT NULL DEFAULT '',
			package_type TEXT NOT NULL DEFAULT 'appimage',
			installed INTEGER NOT NULL DEFAULT 0,
			size_bytes INTEGER NOT NULL DEFAULT 0,
			repository TEXT NOT NULL,
			install_path TEXT NOT NULL DEFAULT '',
			last_seen_at TEXT NOT NULL,
			UNIQUE(repository, name)
		);
		CREATE TABLE IF NOT EXISTS metadata (
			key TEXT PRIMARY KEY NOT NULL,
			value TEXT NOT NULL
		);`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("inicializar la base am-FUSION: %w", err)
	}
	if err := migrateAMFusionSizeColumn(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrar la base am-FUSION: %w", err)
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO repositories(name, url, enabled) VALUES(?, ?, 0)`, amFusionRepoName, amFusionRepoURL); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO repositories(name, url, enabled) VALUES('FPM-local', 'local AppImages', 1)`); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := os.Chmod(path, 0600); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("proteger la base am-FUSION: %w", err)
	}
	return db, nil
}

func migrateAMFusionSizeColumn(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(appimage_packages)`)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, kind string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
			_ = rows.Close()
			return err
		}
		if name == "size_bytes" {
			found = true
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if !found {
		_, err = db.Exec(`ALTER TABLE appimage_packages ADD COLUMN size_bytes INTEGER NOT NULL DEFAULT 0`)
	}
	return err
}

// parseAMFusionSize converts AM's SIZE display to bytes; invalid values are zero.
func parseAMFusionSize(value string) int64 {
	matches := amFusionSizePattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(matches) != 3 {
		return 0
	}
	unit := strings.ToLower(matches[2])
	multiplier := int64(0)
	switch unit {
	case "b", "byte", "bytes":
		multiplier = 1
	case "kib":
		multiplier = 1 << 10
	case "mib":
		multiplier = 1 << 20
	case "gib":
		multiplier = 1 << 30
	case "kb":
		multiplier = 1000
	case "mb":
		multiplier = 1000 * 1000
	case "gb":
		multiplier = 1000 * 1000 * 1000
	default:
		return 0
	}
	amount, ok := new(big.Rat).SetString(matches[1])
	if !ok {
		return 0
	}
	amount.Mul(amount, new(big.Rat).SetInt64(multiplier))
	quotient, remainder := new(big.Int).QuoRem(amount.Num(), amount.Denom(), new(big.Int))
	if new(big.Int).Lsh(remainder, 1).Cmp(amount.Denom()) >= 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	if !quotient.IsInt64() || quotient.Sign() < 0 {
		return 0
	}
	return quotient.Int64()
}

func amFusionIconURL(name string) string {
	slug := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), "-appimage")
	return "https://raw.githubusercontent.com/Portable-Linux-Apps/Portable-Linux-Apps.github.io/main/icons/" + slug + ".png"
}

// stripAMFusionANSI skips the regexp for the (vast majority of) lines that
// contain no escape character.
func stripAMFusionANSI(line string) string {
	if strings.IndexByte(line, 0x1b) < 0 {
		return line
	}
	return amFusionANSI.ReplaceAllString(line, "")
}

func parseAMFusionCatalog(output string) []amFusionPackage {
	seen := make(map[string]bool)
	packages := make([]amFusionPackage, 0)
	for _, raw := range strings.Split(output, "\n") {
		line := strings.TrimSpace(stripAMFusionANSI(raw))
		if !strings.HasPrefix(line, "◆") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "◆"))
		name, description, found := strings.Cut(line, " : ")
		if !found {
			continue
		}
		name = strings.TrimSpace(name)
		description = strings.TrimSpace(description)
		if !amFusionNamePattern.MatchString(name) || seen[name] {
			continue
		}
		seen[name] = true
		packages = append(packages, amFusionPackage{Name: name, Description: description, Type: "appimage"})
	}
	return packages
}

// parseAMFusionInstalled reads AM's documented `-fi --byname` table. The
// parser keys off the stable `appimage` type column, not translated headings.
func parseAMFusionInstalled(output string) map[string]amFusionPackage {
	installed := make(map[string]amFusionPackage)
	for _, raw := range strings.Split(output, "\n") {
		line := strings.TrimSpace(stripAMFusionANSI(raw))
		if !strings.HasPrefix(line, "◆") {
			continue
		}
		columns := strings.Split(line, "|")
		name := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(columns[0]), "◆"))
		name = strings.TrimSuffix(name, "**")
		if !amFusionNamePattern.MatchString(name) {
			continue
		}
		typeColumn := -1
		for i := 1; i < len(columns); i++ {
			kind := strings.ToLower(strings.TrimSpace(columns[i]))
			if strings.Contains(kind, "appimage") {
				typeColumn = i
				break
			}
		}
		if typeColumn < 0 {
			continue
		}
		version := ""
		if typeColumn > 1 {
			version = strings.TrimSpace(columns[typeColumn-1])
		}
		sizeBytes := int64(0)
		if len(columns) > typeColumn+1 {
			sizeBytes = parseAMFusionSize(columns[len(columns)-1])
		}
		installed[name] = amFusionPackage{Name: name, Version: version, Type: "appimage", SizeBytes: sizeBytes, Installed: true}
	}
	return installed
}

func amFusionBackendEnabled() (bool, error) {
	db, err := openAMFusionDB()
	if err != nil {
		return false, err
	}
	defer db.Close()
	var enabled int
	err = db.QueryRow(`SELECT enabled FROM repositories WHERE name=?`, amFusionRepoName).Scan(&enabled)
	return enabled == 1, err
}

func setAMFusionEnabled(enabled bool) error {
	db, err := openAMFusionDB()
	if err != nil {
		return err
	}
	defer db.Close()
	value := 0
	if enabled {
		value = 1
	}
	_, err = db.Exec(`UPDATE repositories SET enabled=? WHERE name=?`, value, amFusionRepoName)
	return err
}

func cacheAMFusionCatalog(packages []amFusionPackage) error {
	if len(packages) == 0 {
		return errors.New("AM no devolvió entradas parseables del catálogo AppImage")
	}
	db, err := openAMFusionDB()
	if err != nil {
		return err
	}
	defer db.Close()
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO appimage_packages(name, description, package_type, installed, repository, last_seen_at)
		VALUES(?, ?, 'appimage', 0, ?, ?)
		ON CONFLICT(repository, name) DO UPDATE SET description=excluded.description, last_seen_at=excluded.last_seen_at`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	for _, pkg := range packages {
		if _, err := stmt.Exec(pkg.Name, pkg.Description, amFusionRepoName, stamp); err != nil {
			_ = stmt.Close()
			_ = tx.Rollback()
			return err
		}
	}
	if err := stmt.Close(); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`UPDATE repositories SET last_sync=? WHERE name=?`, stamp, amFusionRepoName); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func saveAMFusionSnapshot(catalog []amFusionPackage, installed map[string]amFusionPackage) error {
	if len(catalog) == 0 {
		return errors.New("AM no devolvió entradas parseables del catálogo AppImage; se conserva la base anterior")
	}
	db, err := openAMFusionDB()
	if err != nil {
		return err
	}
	defer db.Close()
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM appimage_packages WHERE repository=?`, amFusionRepoName); err != nil {
		_ = tx.Rollback()
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO appimage_packages(name, description, version, package_type, installed, size_bytes, repository, last_seen_at) VALUES(?, ?, ?, 'appimage', ?, ?, ?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	seen := make(map[string]bool, len(catalog)+len(installed))
	for _, pkg := range catalog {
		current := installed[pkg.Name]
		if _, err := stmt.Exec(pkg.Name, pkg.Description, current.Version, boolInt(current.Installed), current.SizeBytes, amFusionRepoName, stamp); err != nil {
			_ = stmt.Close()
			_ = tx.Rollback()
			return err
		}
		seen[pkg.Name] = true
	}
	for name, pkg := range installed {
		if seen[name] {
			continue
		}
		if _, err := stmt.Exec(name, "", pkg.Version, 1, pkg.SizeBytes, amFusionRepoName, stamp); err != nil {
			_ = stmt.Close()
			_ = tx.Rollback()
			return err
		}
	}
	if err := stmt.Close(); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`UPDATE repositories SET last_sync=? WHERE name=?`, stamp, amFusionRepoName); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`INSERT INTO metadata(key, value) VALUES('last_sync', ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, stamp); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func parseAMCommandResult(result RunResult, action string) (string, error) {
	if result.Err != nil {
		if result.Code >= 0 {
			return result.Stdout, fmt.Errorf("AM %s terminó con código %d", action, result.Code)
		}
		return result.Stdout, fmt.Errorf("no se pudo ejecutar AM para %s: %w", action, result.Err)
	}
	return result.Stdout, nil
}

func syncAMFusionDatabase() (int, error) {
	enabled, err := amFusionBackendEnabled()
	if err != nil {
		return 0, err
	}
	if !enabled {
		return 0, fmt.Errorf("el repositorio %s está desactivado", amFusionRepoName)
	}
	catalogOutput, err := parseAMCommandResult(runAMCommand([]string{"-l", "--appimages"}, true), "listar catálogo AppImage")
	if err != nil {
		return 0, err
	}
	installedOutput, err := parseAMCommandResult(runAMCommand([]string{"-fi", "--byname"}, true), "listar apps instaladas")
	if err != nil {
		return 0, err
	}
	catalog := parseAMFusionCatalog(catalogOutput)
	installed := parseAMFusionInstalled(installedOutput)
	if err := saveAMFusionSnapshot(catalog, installed); err != nil {
		return 0, err
	}
	return len(catalog), nil
}

func refreshAMFusionInstalledSnapshot() error {
	_, err := refreshAMFusionInstalledInventory()
	return err
}

func refreshAMFusionInstalledInventory() (map[string]amFusionPackage, error) {
	installedOutput, err := parseAMCommandResult(runAMCommand([]string{"-fi", "--byname"}, true), "listar apps instaladas")
	if err != nil {
		return nil, err
	}
	installed := parseAMFusionInstalled(installedOutput)
	db, err := openAMFusionDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.Exec(`UPDATE appimage_packages SET installed=0, version='', size_bytes=0, last_seen_at=? WHERE repository=?`, stamp, amFusionRepoName); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	for _, pkg := range installed {
		if _, err := tx.Exec(`INSERT INTO appimage_packages(name, description, version, package_type, installed, size_bytes, repository, last_seen_at)
VALUES(?, '', ?, 'appimage', 1, ?, ?, ?)
ON CONFLICT(repository, name) DO UPDATE SET version=excluded.version, installed=1, size_bytes=excluded.size_bytes, last_seen_at=excluded.last_seen_at`,
			pkg.Name, pkg.Version, pkg.SizeBytes, amFusionRepoName, stamp); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}
	if _, err := tx.Exec(`INSERT INTO metadata(key, value) VALUES('inventory_sync', ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, stamp); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return installed, nil
}

func searchAMFusion(query string) (string, error) {
	if strings.TrimSpace(query) == "" {
		return "", errors.New("falta el texto de búsqueda")
	}
	enabled, err := amFusionBackendEnabled()
	if err != nil {
		return "", err
	}
	if !enabled {
		return "", fmt.Errorf("el repositorio %s está desactivado", amFusionRepoName)
	}
	output, err := parseAMCommandResult(runAMCommand([]string{"-q", "--appimages", query}, true), "buscar en am-FUSION")
	if err != nil {
		return output, err
	}
	if packages := parseAMFusionCatalog(output); len(packages) > 0 {
		if err := cacheAMFusionCatalog(packages); err != nil {
			return output, err
		}
	}
	return output, nil
}

func amFusionPackageIndex() (map[string]amFusionPackage, error) {
	db, err := openAMFusionDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT name, description, version, package_type, installed, size_bytes, last_seen_at FROM appimage_packages WHERE repository=?`, amFusionRepoName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	index := make(map[string]amFusionPackage)
	for rows.Next() {
		var pkg amFusionPackage
		var installed int
		if err := rows.Scan(&pkg.Name, &pkg.Description, &pkg.Version, &pkg.Type, &installed, &pkg.SizeBytes, &pkg.UpdatedAt); err != nil {
			return nil, err
		}
		pkg.Installed = installed == 1
		index[strings.ToLower(pkg.Name)] = pkg
	}
	return index, rows.Err()
}

func parseAMFusionSearchItems(output string, cached map[string]amFusionPackage) []TuiItem {
	packages := parseAMFusionCatalog(output)
	items := make([]TuiItem, 0, len(packages))
	for _, pkg := range packages {
		stored := cached[strings.ToLower(pkg.Name)]
		items = append(items, TuiItem{
			Name: pkg.Name, Label: pkg.Name, Desc: pkg.Description,
			Source: amFusionRepoName, Installed: stored.Installed,
			Reinstallable: true, Selectable: true,
		})
	}
	return items
}

func searchAMFusionItems(query string) ([]TuiItem, error) {
	backendAvailable := amBackendAvailable()
	if !backendAvailable && !amFusionDatabaseExists() {
		return nil, nil
	}
	enabled, err := amFusionBackendEnabled()
	if err != nil {
		return nil, err
	}
	if !enabled {
		return nil, nil
	}
	if !backendAvailable {
		return nil, errors.New("AM/AppMan no está disponible en PATH ni en su ruta de instalación local")
	}
	output, err := searchAMFusion(query)
	if err != nil {
		return nil, err
	}
	cached, err := amFusionPackageIndex()
	if err != nil {
		return parseAMFusionSearchItems(output, nil), err
	}
	return parseAMFusionSearchItems(output, cached), nil
}

func listAMFusionInstalled() ([]amFusionPackage, error) {
	if err := refreshAMFusionInstalledSnapshot(); err != nil {
		return nil, err
	}
	db, err := openAMFusionDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT name, description, version, package_type, installed, size_bytes, last_seen_at FROM appimage_packages WHERE repository=? AND installed=1 ORDER BY name`, amFusionRepoName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	packages := make([]amFusionPackage, 0)
	for rows.Next() {
		var pkg amFusionPackage
		var installed int
		if err := rows.Scan(&pkg.Name, &pkg.Description, &pkg.Version, &pkg.Type, &installed, &pkg.SizeBytes, &pkg.UpdatedAt); err != nil {
			return nil, err
		}
		pkg.Installed = installed == 1
		packages = append(packages, pkg)
	}
	return packages, rows.Err()
}

func validateAMFusionRepoName(name string) error {
	if name != "" && name != amFusionRepoName {
		return fmt.Errorf("repositorio desconocido: %s (el integrado es %s)", name, amFusionRepoName)
	}
	return nil
}

func validateAMFusionRepo(name string) error {
	if err := validateAMFusionRepoName(name); err != nil {
		return err
	}
	enabled, err := amFusionBackendEnabled()
	if err != nil {
		return err
	}
	if !enabled {
		return fmt.Errorf("el repositorio %s no está instalado/activo; usa `fpm aimg repo install %s`", amFusionRepoName, amFusionRepoName)
	}
	return nil
}

func amFusionPlainRecord(pkg amFusionPackage) string {
	return fmt.Sprintf("%s\t%s\t%s\t%s\t%d\n", pkg.Name, pkg.Version, pkg.Type, amFusionRepoName, pkg.SizeBytes)
}

func printAMFusionInstalled(plain bool) error {
	packages, err := listAMFusionInstalled()
	if err != nil {
		return err
	}
	if !plain {
		dHeader("AppImages instalados desde %s", amFusionRepoName)
	}
	if len(packages) == 0 && !plain {
		dInfo("No hay AppImages de %s instalados", amFusionRepoName)
	}
	for _, pkg := range packages {
		if plain {
			fmt.Print(amFusionPlainRecord(pkg))
		} else {
			fmt.Printf("  %s*%s %s %s(%s)%s\n", green, reset, pkg.Name, dim, valueOr(pkg.Version, "versión desconocida"), reset)
		}
	}
	return nil
}

func syncFPMLocalAppImages(apps []InstalledApp) error {
	db, err := openAMFusionDB()
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.Exec(`DELETE FROM appimage_packages WHERE repository='FPM-local'`); err != nil {
		_ = tx.Rollback()
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO appimage_packages(name, description, package_type, installed, repository, install_path, last_seen_at)
		VALUES(?, ?, 'appimage', 1, 'FPM-local', ?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	for _, app := range apps {
		name := app.Slug
		if name == "" {
			name = slugify(app.Name)
		}
		if !amFusionNamePattern.MatchString(name) {
			continue
		}
		if _, err := stmt.Exec(name, app.Category, app.AppImagePath, stamp); err != nil {
			_ = stmt.Close()
			_ = tx.Rollback()
			return err
		}
	}
	if err := stmt.Close(); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func refreshFPMLocalAppImageDB() {
	if err := syncFPMLocalAppImages(scanAllAppImages()); err != nil {
		fmt.Fprintf(os.Stderr, "FPM: aviso: no se pudo sincronizar su inventario AppImage local: %v\n", err)
	}
}

func runAMFusionRepoCommand(args []string, backendMode string) {
	if len(args) == 0 {
		args = []string{"list"}
	}
	sub := args[0]
	name := amFusionRepoName
	if len(args) > 1 {
		name = args[1]
	}
	if name != amFusionRepoName {
		dErr("repositorio desconocido: %s (el integrado es %s)", name, amFusionRepoName)
		return
	}
	if backendMode != "" && sub != "add" && sub != "install" {
		dErr("--backend solo se usa con `repo add` o `repo install`")
		return
	}
	switch sub {
	case "list", "status":
		db, err := openAMFusionDB()
		if err != nil {
			dErr("%v", err)
			return
		}
		defer db.Close()
		var enabled int
		var lastSync string
		if err := db.QueryRow(`SELECT enabled, last_sync FROM repositories WHERE name=?`, name).Scan(&enabled, &lastSync); err != nil {
			dErr("%v", err)
			return
		}
		state := "desactivado"
		if enabled == 1 {
			state = "activo"
		}
		fmt.Printf("FPM-local\tactivo\tAppImages instalados localmente\n")
		fmt.Printf("%s\t%s\t%s\n", name, state, amFusionRepoURL)
		fmt.Printf("Base FPM: %s\nÚltima sincronización: %s\n", currentAMFusionDBPath(), valueOr(lastSync, "nunca"))
	case "add", "install":
		if err := ensureAMBackend(backendMode); err != nil {
			dErr("no se pudo instalar o encontrar AM/AppMan: %v", err)
			return
		}
		if backendMode != "" {
			if err := setAMFusionBackend(backendMode); err != nil {
				dErr("no se pudo guardar el backend AM seleccionado: %v", err)
				return
			}
		}
		if err := setAMFusionEnabled(true); err != nil {
			dErr("%v", err)
			return
		}
		count, err := syncAMFusionDatabase()
		if err != nil {
			dErr("AM/AppMan quedó disponible y %s se activó, pero no se pudo sincronizar el catálogo: %v", name, err)
			return
		}
		dOK("Repositorio %s listo: %d paquetes AppImage sincronizados en %s", name, count, currentAMFusionDBPath())
	case "enable", "on":
		if err := setAMFusionEnabled(true); err != nil {
			dErr("%v", err)
			return
		}
		dOK("Repositorio %s activado en FPM", name)
	case "disable", "off":
		if err := setAMFusionEnabled(false); err != nil {
			dErr("%v", err)
			return
		}
		dOK("Repositorio %s desactivado", name)
	case "sync", "update":
		count, err := syncAMFusionDatabase()
		if err != nil {
			dErr("No se pudo sincronizar %s: %v", name, err)
			return
		}
		dOK("%s sincronizado: %d entradas AppImage; base %s", name, count, currentAMFusionDBPath())
	default:
		dErr("subcomando de repositorio desconocido: %s (usa list, add, install, enable, disable o sync)", sub)
	}
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func runAMFusionOperation(args []string, synchronizeInventory bool, eventNames ...[]string) {
	requested := []string(nil)
	if len(eventNames) > 0 {
		requested = eventNames[0]
	}
	eventOperation := aimgEventsEnabled() && len(args) > 0 && (args[0] == "-ia" || args[0] == "-u")
	result := runAMCommandWithProgress(args, requested)
	if result.Err != nil {
		if result.Code >= 0 {
			os.Exit(result.Code)
		}
		dErr("no se pudo ejecutar AM/AppMan: %v", result.Err)
		os.Exit(1)
	}
	if synchronizeInventory {
		installed, err := refreshAMFusionInstalledInventory()
		if err != nil {
			fmt.Fprintf(os.Stderr, "FPM: aviso: AM terminó bien, pero no se pudo sincronizar el inventario: %v\n", err)
		} else if eventOperation {
			emitAMInstalledEvents(os.Stdout, installed, requested)
		}
	}
}

func amFusionFlags(flags []string) []string {
	result := append([]string(nil), flags...)
	if yesFlag && !hasFlag(result, "--assume-yes", "-y") {
		result = append([]string{"-y"}, result...)
	}
	return result
}

func installAMFusionPackages(packages, flags []string) {
	if err := validateAMFusionRepo(amFusionRepoName); err != nil {
		dErr("%v", err)
		return
	}
	args := []string{"-ia"}
	args = append(args, amFusionFlags(flags)...)
	args = append(args, packages...)
	runAMFusionOperation(args, true, packages)
}

func removeAMFusionPackages(packages, flags []string) {
	if err := validateAMFusionRepoName(amFusionRepoName); err != nil {
		dErr("%v", err)
		return
	}
	option := "-r"
	if yesFlag || hasFlag(flags, "--assume-yes", "-y") {
		option = "-R"
	}
	args := []string{option}
	args = append(args, amFusionFlags(flags)...)
	args = append(args, packages...)
	runAMFusionOperation(args, true)
}

func updateAMFusionPackages(packages, flags []string) {
	if err := validateAMFusionRepoName(amFusionRepoName); err != nil {
		dErr("%v", err)
		return
	}
	args := []string{"-u", "--apps"}
	args = append(args, amFusionFlags(flags)...)
	args = append(args, packages...)
	runAMFusionOperation(args, true, packages)
}

func showAMFusionInfo(name string) {
	if err := validateAMFusionRepoName(amFusionRepoName); err != nil {
		dErr("%v", err)
		return
	}
	runAMFusionOperation([]string{"-a", name}, false)
}

func findAMFusionPackage(name string) (amFusionPackage, bool, error) {
	db, err := openAMFusionDB()
	if err != nil {
		return amFusionPackage{}, false, err
	}
	defer db.Close()
	var pkg amFusionPackage
	var installed int
	err = db.QueryRow(`SELECT name, description, version, package_type, installed, size_bytes, last_seen_at FROM appimage_packages WHERE repository=? AND lower(name)=lower(?) LIMIT 1`, amFusionRepoName, name).
		Scan(&pkg.Name, &pkg.Description, &pkg.Version, &pkg.Type, &installed, &pkg.SizeBytes, &pkg.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return amFusionPackage{}, false, nil
	}
	if err != nil {
		return amFusionPackage{}, false, err
	}
	pkg.Installed = installed == 1
	return pkg, true, nil
}
