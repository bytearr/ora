// Package ui holds the Bubble Tea picker.
package cli

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
)

// Interactive reports whether both stdin and stdout are terminals.
func Interactive() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

type model struct {
	title  string
	rows   []string
	cursor int
	chosen int
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch s := k.String(); s {
	case "up", "k", "shift+tab":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j", "tab":
		if m.cursor < len(m.rows)-1 {
			m.cursor++
		}
	case "enter":
		m.chosen = m.cursor
		return m, tea.Quit
	case "esc", "ctrl+c", "q":
		m.chosen = -1
		return m, tea.Quit
	default:
		if len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
			if i := int(s[0] - '1'); i < len(m.rows) {
				m.chosen = i
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m model) View() string {
	if m.chosen != -2 {
		return ""
	}
	var b strings.Builder
	b.WriteString(m.title + "\n\n")
	for i, r := range m.rows {
		cur := "  "
		if i == m.cursor {
			cur = "> "
		}
		fmt.Fprintf(&b, "%s%d  %s\n", cur, i+1, r)
	}
	b.WriteString("\n↑/↓ move · enter launch · 1-8 pick · esc cancel\n")
	return b.String()
}

// Pick shows rows and returns the chosen index, or -1 when cancelled.
func Pick(title string, rows []string) (int, error) {
	res, err := tea.NewProgram(model{title: title, rows: rows, chosen: -2}).Run()
	if err != nil {
		return -1, err
	}
	c := res.(model).chosen
	if c < 0 {
		return -1, nil
	}
	return c, nil
}
