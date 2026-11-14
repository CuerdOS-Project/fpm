package fpm

import (
	"strings"
)

// sourceKind identifica la fuente de un resultado con un entero pequeño. Se
// calcula una sola vez cuando el elemento entra en la lista, de modo que filtrar,
// contar y colorear no comparan cadenas por cada pulsación de tecla.
type sourceKind uint8

const (
	sourceOther sourceKind = iota
	sourceXBPS
	sourceFlatpak
	sourceAM
	sourceAppImage
	sourceKindCount
)

// sourceMask es un conjunto de fuentes (un bit por sourceKind). El valor cero
// significa "todas las fuentes".
type sourceMask uint8

func (m sourceMask) has(kind sourceKind) bool { return m == 0 || m&(1<<kind) != 0 }

func maskOf(kind sourceKind) sourceMask { return 1 << kind }

// searchableKinds lista las fuentes que pueden filtrarse, en el orden en que se
// muestran y en que rota la tecla Tab.
var searchableKinds = [...]sourceKind{sourceXBPS, sourceFlatpak, sourceAM, sourceAppImage}

// sourceDisplayName es el nombre visible de cada fuente (el mismo que usa
// TuiItem.Source).
func sourceDisplayName(kind sourceKind) string {
	switch kind {
	case sourceXBPS:
		return "XBPS"
	case sourceFlatpak:
		return "Flatpak"
	case sourceAM:
		return amFusionRepoName
	case sourceAppImage:
		return "AppImage"
	}
	return "?"
}

// sourceAliases son los nombres que acepta el filtro (`@flatpak`, `@aimg`...).
// El primero de cada lista es el nombre canónico que escribe la tecla Tab.
var sourceAliases = [sourceKindCount][]string{
	sourceXBPS:     {"xbps", "void"},
	sourceFlatpak:  {"flatpak", "flat"},
	sourceAM:       {"am", "am-fusion", "appman"},
	sourceAppImage: {"appimage", "aimg", "local"},
}

// kindOfSource traduce TuiItem.Source (sin distinguir mayúsculas y sin
// reservar memoria) a su sourceKind.
func kindOfSource(source string) sourceKind {
	for kind := sourceXBPS; kind < sourceKindCount; kind++ {
		if strings.EqualFold(source, sourceDisplayName(kind)) {
			return kind
		}
	}
	if strings.EqualFold(source, "AM") {
		return sourceAM
	}
	return sourceOther
}

// parseSourceName acepta un nombre exacto o un prefijo no ambiguo ("@fl" es
// Flatpak; "@a" es ambiguo entre am y appimage y no se acepta).
func parseSourceName(name string) (sourceKind, bool) {
	if name == "" {
		return sourceOther, false
	}
	found, prefixKind := 0, sourceOther
	for kind := sourceXBPS; kind < sourceKindCount; kind++ {
		matched := false
		for _, alias := range sourceAliases[kind] {
			if strings.EqualFold(alias, name) {
				return kind, true
			}
			if len(name) <= len(alias) && strings.EqualFold(alias[:len(name)], name) {
				matched = true
			}
		}
		if matched {
			found++
			prefixKind = kind
		}
	}
	return prefixKind, found == 1
}

// parseSourceList interpreta "xbps,flatpak". Falla si algún nombre no existe.
func parseSourceList(list string) (sourceMask, bool) {
	var mask sourceMask
	for _, name := range strings.Split(list, ",") {
		kind, ok := parseSourceName(strings.TrimSpace(name))
		if !ok {
			return 0, false
		}
		mask |= maskOf(kind)
	}
	return mask, mask != 0
}

// itemFilter es una consulta ya interpretada: qué fuentes se admiten y qué
// palabras deben aparecer (todas) en el nombre, la etiqueta o la descripción.
//
// Sintaxis, válida en la TUI y en `fpm search`:
//
//	firefox                  texto en todas las fuentes
//	@flatpak firefox         solo Flatpak
//	@flatpak:firefox         igual, sin espacio (en la TUI Espacio marca paquetes)
//	@xbps,am:browser         varias fuentes
//
// Un "@algo" que no es una fuente conocida se trata como texto.
type itemFilter struct {
	mask  sourceMask
	terms []string
}

func compileItemFilter(query string) itemFilter {
	var f itemFilter
	for _, field := range strings.Fields(query) {
		if field[0] == '@' {
			head, tail, _ := strings.Cut(field[1:], ":")
			if mask, ok := parseSourceList(head); ok {
				f.mask |= mask
				if tail != "" {
					f.terms = append(f.terms, tail)
				}
				continue
			}
		}
		f.terms = append(f.terms, field)
	}
	return f
}

// text devuelve el texto de búsqueda sin las marcas de fuente.
func (f itemFilter) text() string { return strings.Join(f.terms, " ") }

// matchesText comprueba solo el texto (sin la fuente). No reserva memoria.
func (f itemFilter) matchesText(item TuiItem) bool {
	for _, term := range f.terms {
		if !itemMatches(item, term) {
			return false
		}
	}
	return true
}

// withSourceMask devuelve la consulta con las marcas de fuente sustituidas por
// `mask` (cero = quitarlas), conservando el texto.
func withSourceMask(query string, mask sourceMask) string {
	f := compileItemFilter(query)
	text := f.text()
	if mask == 0 {
		return text
	}
	names := make([]string, 0, int(sourceKindCount))
	for _, kind := range searchableKinds {
		if mask&maskOf(kind) != 0 {
			names = append(names, sourceAliases[kind][0])
		}
	}
	return "@" + strings.Join(names, ",") + ":" + text
}

// nextSourceMask rota Todas → cada fuente presente → Todas. `present` indica
// qué fuentes tienen resultados; así Tab nunca cae en una fuente vacía.
func nextSourceMask(current sourceMask, present [sourceKindCount]bool, backwards bool) sourceMask {
	cycle := make([]sourceMask, 0, len(searchableKinds)+1)
	cycle = append(cycle, 0)
	for _, kind := range searchableKinds {
		if present[kind] || current == maskOf(kind) {
			cycle = append(cycle, maskOf(kind))
		}
	}
	at := 0
	for i, m := range cycle {
		if m == current {
			at = i
			break
		}
	}
	if backwards {
		return cycle[(at+len(cycle)-1)%len(cycle)]
	}
	return cycle[(at+1)%len(cycle)]
}
