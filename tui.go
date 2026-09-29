package main

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
}

func itemMatches(item TuiItem, query string) bool {
	if query == "" {
		return true
	}
	q := strings.ToLower(query)
	return strings.Contains(strings.ToLower(item.Name), q) || strings.Contains(strings.ToLower(item.Label), q) || strings.Contains(strings.ToLower(item.Desc), q)
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

// selectItems implementa una TUI deliberadamente pequeña y testeable. No usa
// memoria manual ni invoca curses; tcell encapsula las diferencias del terminal.
// El estado opcional permite volver a abrirla donde el usuario la dejó.
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
	for {
		filtered := make([]int, 0, len(items))
		for i, item := range items {
			if itemMatches(item, string(query)) {
				filtered = append(filtered, i)
			}
		}
		if cursor >= len(filtered) {
			cursor = maxInt(0, len(filtered)-1)
		}
		width, height := screen.Size()
		listRows := maxInt(1, height-6)
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
			line := fmtTui("%s %-10s %-28s %s", marker, item.Source, firstNonEmpty(item.Label, item.Name), item.Desc)
			if item.Installed {
				line += " [" + tr("tui_installed") + "]"
			}
			lineStyle := style
			if pos == cursor {
				lineStyle = tcell.StyleDefault.Background(tcell.ColorBlue).Foreground(tcell.ColorWhite).Bold(true)
			}
			drawText(screen, 0, 4+row, width, line, lineStyle)
		}
		drawText(screen, 0, height-1, width, " "+tr("tui_help"), tcell.StyleDefault.Background(tcell.ColorYellow).Foreground(tcell.ColorBlack))

		screen.Show()
		event := screen.PollEvent()
		switch ev := event.(type) {
		case *tuiItemsEvent:
			items = append(items, ev.items...)
			selected = append(selected, make([]bool, len(ev.items))...)
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
			case tcell.KeyEscape:
				saveResume()
				return nil, nil
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
			case tcell.KeyBackspace, tcell.KeyBackspace2:

				if len(query) > 0 {
					query = query[:len(query)-1]
					cursor = 0
					offset = 0
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
				r := ev.Rune()
				if r == 'q' || r == 'Q' {
					saveResume()
					return nil, nil
				}

				if r == ' ' && len(filtered) > 0 {
					idx := filtered[cursor]
					if items[idx].Selectable {
						selected[idx] = !selected[idx]
					}
				} else if r == 'a' || r == 'A' {
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
				} else if r >= 32 && r != utf8.RuneError {
					query = append(query, r)
					cursor = 0
					offset = 0
				}
			}
		}
	}
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
