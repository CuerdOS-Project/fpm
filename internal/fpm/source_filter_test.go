package fpm

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCompileItemFilter(t *testing.T) {
	cases := []struct {
		query string
		mask  sourceMask
		terms []string
	}{
		{"firefox", 0, []string{"firefox"}},
		{"@flatpak firefox", maskOf(sourceFlatpak), []string{"firefox"}},
		{"@flatpak:firefox", maskOf(sourceFlatpak), []string{"firefox"}},
		{"@xbps,am:browser", maskOf(sourceXBPS) | maskOf(sourceAM), []string{"browser"}},
		{"@aimg", maskOf(sourceAppImage), nil},
		{"@fl:gimp", maskOf(sourceFlatpak), []string{"gimp"}}, // prefijo único
		{"@a:gimp", 0, []string{"@a:gimp"}},                   // ambiguo (am/appimage): es texto
		{"@desconocida foo", 0, []string{"@desconocida", "foo"}},
		{"web browser", 0, []string{"web", "browser"}},
		{"", 0, nil},
	}
	for _, tc := range cases {
		f := compileItemFilter(tc.query)
		if f.mask != tc.mask || !reflect.DeepEqual(f.terms, tc.terms) {
			t.Errorf("compileItemFilter(%q) = {%b %v}, want {%b %v}", tc.query, f.mask, f.terms, tc.mask, tc.terms)
		}
	}
}

func TestKindOfSource(t *testing.T) {
	for source, want := range map[string]sourceKind{
		"XBPS": sourceXBPS, "Flatpak": sourceFlatpak, "am-FUSION": sourceAM,
		"AM": sourceAM, "AppImage": sourceAppImage, "otra": sourceOther, "": sourceOther,
	} {
		if got := kindOfSource(source); got != want {
			t.Errorf("kindOfSource(%q) = %d, want %d", source, got, want)
		}
	}
}

func TestMatchesTextRequiresEveryWordAndDoesNotAllocate(t *testing.T) {
	item := TuiItem{Name: "firefox", Label: "firefox", Desc: "Mozilla Web Browser"}
	if !compileItemFilter("@xbps web mozilla").matchesText(item) {
		t.Error("todas las palabras están en la descripción")
	}
	if compileItemFilter("web chromium").matchesText(item) {
		t.Error("falta una palabra: no debe coincidir")
	}
	f := compileItemFilter("@flatpak web browser")
	if allocs := testing.AllocsPerRun(100, func() { f.matchesText(item) }); allocs != 0 {
		t.Errorf("matchesText reserva memoria (%.0f); se ejecuta por cada elemento y tecla", allocs)
	}
}

func TestWithSourceMask(t *testing.T) {
	cases := []struct {
		query string
		mask  sourceMask
		want  string
	}{
		{"", maskOf(sourceFlatpak), "@flatpak:"},
		{"firefox", maskOf(sourceXBPS), "@xbps:firefox"},
		{"@flatpak:firefox", maskOf(sourceAM), "@am:firefox"},
		{"@xbps,flatpak:gimp", 0, "gimp"},
		{"@xbps", 0, ""},
		{"web browser", maskOf(sourceAppImage), "@appimage:web browser"},
	}
	for _, tc := range cases {
		if got := withSourceMask(tc.query, tc.mask); got != tc.want {
			t.Errorf("withSourceMask(%q) = %q, want %q", tc.query, got, tc.want)
		}
		// El resultado debe volver a interpretarse como la misma máscara.
		if got := compileItemFilter(withSourceMask(tc.query, tc.mask)).mask; got != tc.mask {
			t.Errorf("ida y vuelta de %q: máscara %b, want %b", tc.query, got, tc.mask)
		}
	}
}

func TestNextSourceMaskSkipsEmptySources(t *testing.T) {
	present := [sourceKindCount]bool{sourceXBPS: true, sourceFlatpak: true, sourceAppImage: true}
	var seen []sourceMask
	m := sourceMask(0)
	for i := 0; i < 4; i++ {
		m = nextSourceMask(m, present, false)
		seen = append(seen, m)
	}
	want := []sourceMask{maskOf(sourceXBPS), maskOf(sourceFlatpak), maskOf(sourceAppImage), 0}
	if !reflect.DeepEqual(seen, want) {
		t.Errorf("ciclo hacia delante = %v, want %v", seen, want)
	}
	if got := nextSourceMask(0, present, true); got != maskOf(sourceAppImage) {
		t.Errorf("Shift+Tab desde Todas = %b, want AppImage", got)
	}
	// Una máscara múltiple escrita a mano no está en el ciclo: vuelve al principio.
	if got := nextSourceMask(maskOf(sourceXBPS)|maskOf(sourceFlatpak), present, false); got != maskOf(sourceXBPS) {
		t.Errorf("máscara múltiple + Tab = %b, want XBPS", got)
	}
}

func TestSearchQueryFromArgs(t *testing.T) {
	cases := []struct {
		args    []string
		want    string
		wantErr bool
	}{
		{[]string{"firefox"}, "firefox", false},
		{[]string{"--source", "flatpak", "firefox"}, "@flatpak firefox", false},
		{[]string{"firefox", "--source=xbps,am"}, "@xbps,am firefox", false},
		{[]string{"@flatpak", "firefox"}, "@flatpak firefox", false},
		{[]string{"-v", "firefox"}, "firefox", false},
		{[]string{"@flatpak"}, "", false},
		{[]string{"--source", "flatpak"}, "", false},
		{[]string{"--source", "snap", "firefox"}, "", true},
		{[]string{"--source"}, "", true},
		{nil, "", false},
	}
	for _, tc := range cases {
		got, err := searchQueryFromArgs(tc.args)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("searchQueryFromArgs(%v) = %q, %v; want %q (error=%v)", tc.args, got, err, tc.want, tc.wantErr)
		}
	}
}

// Con el filtro @flatpak no debe ni ejecutarse xbps-query: la eficiencia está
// en no consultar las fuentes excluidas.
func TestSearchAllOnlyQueriesRequestedSources(t *testing.T) {
	binDir := t.TempDir()
	marker := filepath.Join(binDir, "xbps-called")
	writeSearchTestCommand(t, binDir, "xbps-query", "#!/bin/sh\n: > '"+marker+"'\nprintf '[*] firefox-128.0_1 XBPS web browser\\n'\n")
	writeSearchTestCommand(t, binDir, "xbps-install", "#!/bin/sh\nexit 0\n")
	writeSearchTestCommand(t, binDir, "flatpak", `#!/bin/sh
case "$1" in
  search) printf 'Firefox\tFlatpak web browser\torg.mozilla.firefox\tflathub\n' ;;
  list) ;;
esac
`)
	t.Setenv("PATH", binDir)

	_, output := captureSearchStdout(t, func() int { searchAll("@flatpak:firefox"); return 0 })
	if !strings.Contains(output, "[Flatpak  ]") || !strings.Contains(output, "Firefox") {
		t.Fatalf("falta el resultado de Flatpak:\n%s", output)
	}
	if strings.Contains(output, "XBPS") {
		t.Fatalf("@flatpak no debe mostrar XBPS:\n%s", output)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("xbps-query se ejecutó aunque XBPS estaba excluida")
	}

	_, output = captureSearchStdout(t, func() int { searchAll("@xbps,flatpak firefox"); return 0 })
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("la comprobación del marcador es inválida: xbps-query no dejó rastro cuando sí debía ejecutarse")
	}
	xbpsAt, flatpakAt := strings.Index(output, "[XBPS"), strings.Index(output, "[Flatpak")
	if xbpsAt < 0 || flatpakAt < 0 || xbpsAt > flatpakAt {
		t.Fatalf("se esperaban XBPS y luego Flatpak, agrupados por fuente:\n%s", output)
	}
}

func TestTruncateAndWrapRunes(t *testing.T) {
	if got := truncateRunes("abcdefgh", 5); got != "abcd…" {
		t.Errorf("truncateRunes = %q", got)
	}
	if got := truncateRunes("abc", 5); got != "abc" {
		t.Errorf("truncateRunes corto = %q", got)
	}
	got := wrapRunes("uno dos tres cuatro cinco seis siete", 10, 2)
	if len(got) != 2 || got[0] != "uno dos" || !strings.HasSuffix(got[1], "…") {
		t.Errorf("wrapRunes = %q", got)
	}
	for _, line := range wrapRunes("palabrasuperlargasinespacios y más", 8, 3) {
		if len([]rune(line)) > 8 {
			t.Errorf("línea excede el ancho: %q", line)
		}
	}
}
