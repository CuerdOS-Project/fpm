package fpm

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
)

type TuiItem struct {
	Name          string
	Label         string
	Desc          string
	Source        string
	Remote        string
	Installed     bool
	Reinstallable bool
	Selectable    bool

	// kind se deriva de Source al entrar en la TUI; evita comparar cadenas al
	// filtrar, contar y colorear.
	kind sourceKind
}

func itemMatches(item TuiItem, query string) bool {
	if query == "" {
		return true
	}
	// containsFold compares ASCII text without allocating: this runs for every
	// item on every keystroke.
	return containsFold(item.Name, query) || containsFold(item.Label, query) || containsFold(item.Desc, query)
}

func drawText(screen tcell.Screen, x, y, width int, text string, style tcell.Style) {
	if width <= 0 || y < 0 {
		return
	}
	runes := []rune(text)
	if len(runes) > width {
		runes = runes[:width]
	}
	for _, r := range runes {
		screen.SetContent(x, y, r, nil, style)
		x++
	}
}

type TuiResume struct {
	Query  string
	Cursor int
}

type tuiItemsEvent struct{ items []TuiItem }
type tuiItemsDoneEvent struct{}

func (e *tuiItemsEvent) When() time.Time     { return time.Now() }
func (e *tuiItemsDoneEvent) When() time.Time { return time.Now() }

// sourceStyle da a cada fuente un color propio para reconocerla de un vistazo.
func sourceStyle(kind sourceKind) tcell.Style {
	base := tcell.StyleDefault
	switch kind {
	case sourceXBPS:
		return base.Foreground(tcell.ColorGreen)
	case sourceFlatpak:
		return base.Foreground(tcell.ColorAqua)
	case sourceAM:
		return base.Foreground(tcell.ColorFuchsia)
	case sourceAppImage:
		return base.Foreground(tcell.ColorOrange)
	}
	return base.Foreground(tcell.ColorWhite)
}

// tuiChip es una pestaña de fuente de la fila 3; guarda su zona de clic.
type tuiChip struct {
	x0, x1 int
	mask   sourceMask
}

// selectItems implementa una TUI deliberadamente pequeña y testeable. No usa
// memoria manual ni invoca curses; tcell encapsula las diferencias del terminal.
// El estado opcional permite volver a abrirla donde el usuario la dejó.
//
// El filtro (fila 1) admite `@fuente` además de texto, y la fila 3 muestra una
// pestaña por fuente con su número de coincidencias (Tab, Shift+Tab o clic para
// cambiar). La lista filtrada y los contadores se recalculan en un único
// recorrido y solo cuando cambian el texto o los elementos, no en cada evento.
func selectItems(title string, items []TuiItem, allowActivation bool, resume *TuiResume, updates ...<-chan []TuiItem) ([]TuiItem, *TuiItem) {
	screen, err := tcell.NewScreen()
	if err != nil {
		return nil, nil
	}
	if err := screen.Init(); err != nil {
		return nil, nil
	}
	defer screen.Fini()
	screen.EnableMouse()
	selected := make([]bool, len(items))
	var present [sourceKindCount]bool
	for i := range items {
		items[i].kind = kindOfSource(items[i].Source)
		present[items[i].kind] = true
	}
	var updateCh <-chan []TuiItem
	if len(updates) > 0 {
		updateCh = updates[0]
	}
	loading := updateCh != nil
	if updateCh != nil {
		go func() {
			for batch := range updateCh {
				_ = screen.PostEvent(&tuiItemsEvent{items: batch})
			}
			_ = screen.PostEvent(&tuiItemsDoneEvent{})
		}()
	}

	query := []rune{}

	cursor, offset := 0, 0
	if resume != nil {
		query = []rune(resume.Query)
		cursor = resume.Cursor
	}
	saveResume := func() {
		if resume != nil {
			resume.Query = string(query)
			resume.Cursor = cursor
		}
	}

	var (
		flt         itemFilter
		filtered    = make([]int, 0, len(items))
		matchCounts [sourceKindCount]int // coincidencias de texto por fuente
		textTotal   int
		dirty       = true
		chips       []tuiChip
		maxLabel    int // etiqueta más larga entre los resultados visibles
	)
	refilter := func() {
		flt = compileItemFilter(string(query))
		filtered = filtered[:0]
		matchCounts = [sourceKindCount]int{}
		textTotal, maxLabel = 0, 0
		for i := range items {
			if !flt.matchesText(items[i]) {
				continue
			}
			matchCounts[items[i].kind]++
			textTotal++
			if flt.mask.has(items[i].kind) {
				filtered = append(filtered, i)
				if n := utf8.RuneCountInString(firstNonEmpty(items[i].Label, items[i].Name)); n > maxLabel {
					maxLabel = n
				}
			}
		}
		dirty = false
	}
	setSourceMask := func(mask sourceMask) {
		query = []rune(withSourceMask(string(query), mask))
		cursor, offset = 0, 0
		dirty = true
	}
	for {
		if dirty {
			refilter()
		}
		if cursor >= len(filtered) {
			cursor = maxInt(0, len(filtered)-1)
		}
		width, height := screen.Size()
		// Panel de detalle (3 filas) sobre la ayuda cuando hay altura suficiente:
		// muestra completo el paquete bajo el cursor.
		detailH := 0
		if height >= 15 {
			detailH = 3
		}
		detailTop := height - 1 - detailH
		listRows := maxInt(1, detailTop-4)
		// Columnas adaptables: el nombre usa lo que necesita (con tope) y la
		// descripción el resto; ambas se recortan con "…" en vez de pisarse.
		nameW := minInt(maxInt(maxLabel, 12), maxInt(12, width*40/100))
		descX := 14 + nameW + 1
		if cursor < offset {
			offset = cursor
		}
		if cursor >= offset+listRows {
			offset = cursor - listRows + 1
		}
		screen.Clear()
		style := tcell.StyleDefault.Foreground(tcell.ColorWhite)
		drawText(screen, 0, 0, width, " "+title, tcell.StyleDefault.Foreground(tcell.NewRGBColor(0, 200, 200)).Bold(true))
		drawText(screen, 0, 1, width, " "+tr("tui_filter_label")+": "+string(query)+"▌", tcell.StyleDefault.Foreground(tcell.ColorYellow))
		status := fmtTui(" %d %s | %d %s", len(filtered), tr("tui_results"), countSelected(selected), tr("tui_selected"))
		if loading {
			status += " | " + tr("tui_loading_sources")
		}
		drawText(screen, 0, 2, width, status, style)

		// Fila 3: pestañas de fuente con contador.
		chips = chips[:0]
		x := 1
		drawChip := func(label string, count int, mask sourceMask, kind sourceKind) {
			text := fmtTui(" %s %d ", label, count)
			chipStyle := sourceStyle(kind)
			active := flt.mask == mask
			if mask != 0 && flt.mask&mask != 0 {
				active = true
			}
			if active {
				fg, _, _ := chipStyle.Decompose()
				chipStyle = tcell.StyleDefault.Background(fg).Foreground(tcell.ColorBlack).Bold(true)
			}
			w := utf8.RuneCountInString(text)
			drawText(screen, x, 3, width-x, text, chipStyle)
			chips = append(chips, tuiChip{x0: x, x1: x + w, mask: mask})
			x += w + 1
		}
		drawChip(tr("tui_all_sources"), textTotal, 0, sourceOther)
		for _, kind := range searchableKinds {
			if present[kind] {
				drawChip(sourceDisplayName(kind), matchCounts[kind], maskOf(kind), kind)
			}
		}

		for row := 0; row < listRows && row+offset < len(filtered); row++ {
			pos := row + offset
			item := items[filtered[pos]]
			marker := "[ ]"
			if selected[filtered[pos]] {
				marker = "[x]"
			}
			if !item.Selectable {
				marker = " - "
			}
			label := truncateRunes(firstNonEmpty(item.Label, item.Name), nameW)
			desc := item.Desc
			if item.Installed {
				desc += " [" + tr("tui_installed") + "]"
			}
			lineStyle, badgeStyle := style, sourceStyle(item.kind)
			if pos == cursor {
				lineStyle = tcell.StyleDefault.Background(tcell.ColorBlue).Foreground(tcell.ColorWhite).Bold(true)
				badgeStyle = lineStyle
			}
			// Con el cursor encima se pinta toda la fila para que el resaltado sea continuo.
			drawText(screen, 0, 4+row, width, strings.Repeat(" ", width), lineStyle)
			drawText(screen, 0, 4+row, width, marker, lineStyle)
			drawText(screen, 4, 4+row, width-4, fmtTui("%-9s", item.Source), badgeStyle.Bold(true))
			drawText(screen, 14, 4+row, width-14, label, lineStyle)
			drawText(screen, descX, 4+row, width-descX, truncateRunes(desc, width-descX), lineStyle)
		}
		if detailH > 0 {
			detailStyle := tcell.StyleDefault.Foreground(tcell.ColorWhite)
			drawText(screen, 0, detailTop-0, width, strings.Repeat("─", width), tcell.StyleDefault.Foreground(tcell.ColorGray))
			if cursor < len(filtered) {
				it := items[filtered[cursor]]
				head := firstNonEmpty(it.Label, it.Name)
				if it.Name != "" && it.Name != head {
					head += "  (" + it.Name + ")"
				}
				head += "  · " + it.Source
				if it.Remote != "" {
					head += "/" + it.Remote
				}
				drawText(screen, 1, detailTop+1, width-1, truncateRunes(head, width-1), sourceStyle(it.kind).Bold(true))
				for i, line := range wrapRunes(it.Desc, width-2, detailH-2) {
					drawText(screen, 1, detailTop+2+i, width-2, line, detailStyle)
				}
			}
		}
		drawText(screen, 0, height-1, width, " "+tr("tui_help"), tcell.StyleDefault.Background(tcell.ColorYellow).Foreground(tcell.ColorBlack))

		screen.Show()
		event := screen.PollEvent()
		switch ev := event.(type) {
		case *tuiItemsEvent:
			for i := range ev.items {
				ev.items[i].kind = kindOfSource(ev.items[i].Source)
				present[ev.items[i].kind] = true
			}
			items = append(items, ev.items...)
			selected = append(selected, make([]bool, len(ev.items))...)
			dirty = true
		case *tuiItemsDoneEvent:
			loading = false
		case *tcell.EventResize:

			screen.Sync()
		case *tcell.EventMouse:
			x, y := ev.Position()
			buttons := ev.Buttons()
			if buttons&tcell.WheelUp != 0 {
				cursor = maxInt(0, cursor-3)
			} else if buttons&tcell.WheelDown != 0 {
				cursor = minInt(maxInt(0, len(filtered)-1), cursor+3)
			} else if buttons&tcell.Button2 != 0 {
				for i := range selected {
					selected[i] = false
				}
			} else if y == 3 {
				if buttons&tcell.Button1 != 0 {
					for _, chip := range chips {
						if x >= chip.x0 && x < chip.x1 {
							setSourceMask(chip.mask)
							break
						}
					}
				}
			} else if x >= 0 && y >= 4 && y < 4+listRows && y-4+offset < len(filtered) {
				idxPos := y - 4 + offset
				idx := filtered[idxPos]
				cursor = idxPos
				if buttons&tcell.Button1 != 0 {
					if items[idx].Selectable {
						selected[idx] = true
					}
				} else if buttons&tcell.Button3 != 0 {
					if items[idx].Selectable {
						selected[idx] = false
					}
				}
			}

		case *tcell.EventKey:

			switch ev.Key() {
			case tcell.KeyEscape, tcell.KeyCtrlC:
				saveResume()
				return nil, nil
			case tcell.KeyTab:
				setSourceMask(nextSourceMask(flt.mask, present, false))
			case tcell.KeyBacktab:
				setSourceMask(nextSourceMask(flt.mask, present, true))
			case tcell.KeyUp:
				if cursor > 0 {
					cursor--
				}
			case tcell.KeyDown:
				if cursor < len(filtered)-1 {
					cursor++
				}
			case tcell.KeyPgUp:
				cursor = maxInt(0, cursor-listRows)
			case tcell.KeyPgDn:
				cursor = minInt(maxInt(0, len(filtered)-1), cursor+listRows)
			case tcell.KeyHome:
				cursor = 0
			case tcell.KeyEnd:
				cursor = maxInt(0, len(filtered)-1)
			case tcell.KeyCtrlA:
				// Marca (o desmarca) todo lo que el filtro deja visible.
				all := true
				for _, idx := range filtered {
					if items[idx].Selectable && !selected[idx] {
						all = false
						break
					}
				}
				for _, idx := range filtered {
					if items[idx].Selectable {
						selected[idx] = !all
					}
				}
			case tcell.KeyBackspace, tcell.KeyBackspace2:

				if len(query) > 0 {
					query = query[:len(query)-1]
					cursor = 0
					offset = 0
					dirty = true
				}
			case tcell.KeyEnter:
				if len(filtered) == 0 {
					saveResume()
					return nil, nil
				}
				idx := filtered[cursor]
				if allowActivation && !items[idx].Selectable {
					activated := items[idx]
					saveResume()
					return nil, &activated
				}
				result := make([]TuiItem, 0)
				for i, ok := range selected {
					if ok {
						result = append(result, items[i])
					}
				}
				saveResume()
				return result, nil
			default:
				// Las letras siempre se escriben en el filtro (hacen falta para
				// `@flatpak`); salir es Esc/Ctrl+C y marcar todo es Ctrl+A.
				r := ev.Rune()
				if r == ' ' && len(filtered) > 0 {
					idx := filtered[cursor]
					if items[idx].Selectable {
						selected[idx] = !selected[idx]
					}
				} else if r >= 32 && r != utf8.RuneError {
					query = append(query, r)
					cursor = 0
					offset = 0
					dirty = true
				}
			}
		}
	}
}

// truncateRunes recorta a w celdas añadiendo "…" si no cabe.
func truncateRunes(text string, w int) string {
	if w <= 0 {
		return ""
	}
	if utf8.RuneCountInString(text) <= w {
		return text
	}
	r := []rune(text)
	return string(r[:w-1]) + "…"
}

// wrapRunes parte text en hasta maxLines líneas de w celdas, cortando por
// palabras; la última se recorta con "…" si sobra texto.
func wrapRunes(text string, w, maxLines int) []string {
	if w <= 0 || maxLines <= 0 {
		return nil
	}
	var lines []string
	cur := ""
	words := strings.Fields(text)
	for i, word := range words {
		next := word
		if cur != "" {
			next = cur + " " + word
		}
		if utf8.RuneCountInString(next) <= w {
			cur = next
			continue
		}
		if cur != "" {
			lines = append(lines, cur)
			cur = ""
		}
		if len(lines) == maxLines {
			lines[maxLines-1] = truncateRunes(lines[maxLines-1]+" "+strings.Join(words[i:], " "), w)
			return lines
		}
		cur = truncateRunes(word, w)
	}
	if cur != "" {
		if len(lines) == maxLines {
			lines[maxLines-1] = truncateRunes(lines[maxLines-1]+" "+cur, w)
		} else {
			lines = append(lines, cur)
		}
	}
	return lines
}

func selectedTuiItems(items []TuiItem, selected []bool) []TuiItem {
	result := make([]TuiItem, 0)
	for i, ok := range selected {
		if ok && i < len(items) {
			result = append(result, items[i])
		}
	}
	return result
}

func countSelected(selected []bool) int {
	n := 0
	for _, ok := range selected {
		if ok {
			n++
		}
	}
	return n
}
func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func fmtTui(format string, args ...any) string { return fmt.Sprintf(format, args...) }
