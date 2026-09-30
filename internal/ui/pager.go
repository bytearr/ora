package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

const PageSize = 20

type pager struct {
	header string
	rows   []string
	page   int
	width  int
}

func (p pager) pages() int {
	return max(1, (len(p.rows)+PageSize-1)/PageSize)
}

func (p pager) Init() tea.Cmd { return nil }

func (p pager) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width = msg.Width
	case tea.KeyMsg:
		switch msg.String() {
		case "right", "l", "pgdown", "space", "n":
			p.page = min(p.page+1, p.pages()-1)
		case "left", "h", "pgup", "p":
			p.page = max(p.page-1, 0)
		case "home", "g":
			p.page = 0
		case "end", "G":
			p.page = p.pages() - 1
		case "q", "esc", "ctrl+c":
			return p, tea.Quit
		}
	}
	return p, nil
}

func (p pager) fit(s string) string {
	if p.width <= 0 {
		return s
	}
	return ansi.Truncate(s, p.width, "…")
}

func (p pager) View() string {
	var b strings.Builder
	b.WriteString(p.fit(p.header) + "\n")
	start := p.page * PageSize
	end := min(start+PageSize, len(p.rows))
	for _, r := range p.rows[start:end] {
		b.WriteString(p.fit(r) + "\n")
	}
	for i := end - start; i < PageSize; i++ {
		b.WriteString("\n")
	}
	b.WriteString(p.fit(fmt.Sprintf("\nPage %d/%d · %d entries · ←/→ page · Home/End · q quit",
		p.page+1, p.pages(), len(p.rows))))
	return b.String()
}

// Page shows pre-formatted rows 20 at a time on the alternate screen.
func Page(header string, rows []string) error {
	_, err := tea.NewProgram(pager{header: header, rows: rows}, tea.WithAltScreen()).Run()
	return err
}
