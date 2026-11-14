package fpm

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	amInstallerReleaseURL = "https://github.com/ivan-hc/AM/releases/download/10.6/AM-INSTALLER"
	amInstallerSHA256     = "ecb75ab49e6bccbf448c889c029cdd5f9234908f24054638fe2d6db9dada2d07"
	amInstallerMaxSize    = 64 * 1024
)

func appManInstallPath() string {
	binDir := os.Getenv("XDG_BIN_HOME")
	if binDir == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			home = os.Getenv("HOME")
		}
		if home == "" {
			return ""
		}
		binDir = filepath.Join(home, ".local", "bin")
	}
	if !filepath.IsAbs(binDir) {
		absolute, err := filepath.Abs(binDir)
		if err != nil {
			return ""
		}
		binDir = absolute
	}
	return filepath.Join(binDir, "appman")
}

func amBackendAvailable() bool {
	_, err := resolveAMBackend()
	return err == nil
}

func amBackendCommandAvailable(mode string) bool {
	if _, err := exec.LookPath(mode); err == nil {
		return true
	}
	if mode == "appman" {
		path := appManInstallPath()
		info, err := os.Stat(path)
		return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0
	}
	return false
}

func verifyAMInstallerPayload(payload []byte) error {
	if len(payload) == 0 || len(payload) > amInstallerMaxSize {
		return fmt.Errorf("tamaño inesperado para AM-INSTALLER 10.6: %d bytes", len(payload))
	}
	digest := sha256.Sum256(payload)
	if got := hex.EncodeToString(digest[:]); got != amInstallerSHA256 {
		return fmt.Errorf("SHA-256 incorrecto para AM-INSTALLER 10.6: %s", got)
	}
	return nil
}

func downloadAMInstaller() (string, func(), error) {
	client := &http.Client{Timeout: 90 * time.Second}
	request, err := http.NewRequest(http.MethodGet, amInstallerReleaseURL, nil)
	if err != nil {
		return "", nil, err
	}
	request.Header.Set("User-Agent", "FPM-AM-INSTALLER/2.2")
	response, err := client.Do(request)
	if err != nil {
		return "", nil, fmt.Errorf("descargar AM-INSTALLER 10.6: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("descargar AM-INSTALLER 10.6: HTTP %s", response.Status)
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, amInstallerMaxSize+1))
	if err != nil {
		return "", nil, fmt.Errorf("leer AM-INSTALLER 10.6: %w", err)
	}
	if err := verifyAMInstallerPayload(payload); err != nil {
		return "", nil, err
	}
	workDir, err := os.MkdirTemp("", "fpm-am-installer-*")
	if err != nil {
		return "", nil, fmt.Errorf("crear directorio temporal para AM-INSTALLER: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(workDir) }
	path := filepath.Join(workDir, "AM-INSTALLER")
	if err := os.WriteFile(path, payload, 0700); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("guardar AM-INSTALLER verificado: %w", err)
	}
	return path, cleanup, nil
}

func ensureAMBackend(mode string) error {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode != "" && mode != "am" && mode != "appman" {
		return fmt.Errorf("backend desconocido %q; usa `--backend am` o `--backend appman`", mode)
	}
	if mode == "" {
		if amBackendAvailable() {
			return nil
		}
		if !isTTY() {
			return errors.New("AM/AppMan no está instalado; en modo no interactivo indica `--backend am` o `--backend appman`")
		}
	} else if amBackendCommandAvailable(mode) {
		return nil
	}

	installerPath, cleanup, err := downloadAMInstaller()
	if err != nil {
		return err
	}
	defer cleanup()
	args := []string{}
	if mode != "" {
		args = []string{"--install", mode}
	}
	fmt.Println("Se verificó AM-INSTALLER 10.6 con SHA-256 antes de ejecutarlo.")
	command := exec.Command(installerPath, args...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("AM-INSTALLER terminó con error: %w", err)
	}
	if mode != "" {
		if !amBackendCommandAvailable(mode) {
			return fmt.Errorf("AM-INSTALLER terminó, pero no se encontró el comando %q", mode)
		}
	} else if !amBackendAvailable() {
		return errors.New("AM-INSTALLER terminó sin dejar disponible `am` ni `appman`")
	}
	return nil
}
