package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"unicode"
)

const (
	headerHeight = 3
	footerHeight = 3
)

var (
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ff5f87")).Background(lipgloss.Color("#0f0f0f")).Padding(0, 1)
	viewportStyle = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#7fdbca")).Padding(0, 1).Margin(0, 1)
	statusStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#8ad6ff")).Background(lipgloss.Color("#121212")).Padding(0, 1)
	commandStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#f4b8e4")).Bold(true)
	helpStyle     = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#f4b8e4")).Padding(1, 2)
)

type pdfModel struct {
	path          string
	pages         []string
	viewport      viewport.Model
	currentPage   int
	totalPages    int
	mode          string
	statusMessage string
	helpMessage   string
	commandBuffer string
	commandMode   bool
	showHelp      bool
	matches       []int
	matchIndex    int
	searchTerm    string
}

func newPDFModel(path string, pages []string) pdfModel {
	vp := viewport.New(80, 20)
	model := pdfModel{
		path:          path,
		pages:         pages,
		viewport:      vp,
		currentPage:   1,
		totalPages:    len(pages),
		mode:          "NORMAL",
		statusMessage: "Press : for commands, / to search",
		helpMessage:   "help | goto <n> | search <term> | export",
	}
	model.refreshPage()
	model.viewport.GotoTop()
	return model
}

func (m pdfModel) Init() tea.Cmd {
	return nil
}

func (m pdfModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		height := msg.Height - headerHeight - footerHeight
		if height < 6 {
			height = 6
		}
		width := msg.Width - 6
		if width < 50 {
			width = 50
		}
		m.viewport.Width = width
		m.viewport.Height = height
		m.refreshPage()
		m.viewport.GotoTop()
		return m, nil
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC || msg.String() == "q" {
			return m, tea.Quit
		}
		if m.showHelp {
			if msg.Type == tea.KeyEsc || msg.String() == "?" {
				m.showHelp = false
				m.mode = "NORMAL"
				m.statusMessage = "Help closed"
				return m, nil
			}
		}
		if m.commandMode {
			return m.handleCommandMode(msg)
		}
		return m.handleNavigation(msg)
	}
	return m, nil
}

func (m pdfModel) handleCommandMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.commandMode = false
		m.commandBuffer = ""
		m.mode = "NORMAL"
		m.showHelp = false
		m.statusMessage = "Command cancelled"
		return m, nil
	case tea.KeyEnter:
		m.commandMode = false
		input := strings.TrimSpace(m.commandBuffer)
		m.commandBuffer = ""
		m.mode = "NORMAL"
		m.processCommand(input)
		return m, nil
	case tea.KeyBackspace, tea.KeyDelete:
		if len(m.commandBuffer) > 0 {
			m.commandBuffer = m.commandBuffer[:len(m.commandBuffer)-1]
		}
		return m, nil
	default:
		if msg.Type == tea.KeyRunes {
			m.commandBuffer += msg.String()
		}
		return m, nil
	}
}

func (m *pdfModel) processCommand(input string) {
	switch {
	case input == "":
		m.statusMessage = "No command entered"
	case input == "tools", input == "help", input == "?", input == ":help", input == "commands":
		m.showHelp = true
		m.mode = "HELP"
		m.statusMessage = "Help opened (Esc to close)"
	case strings.HasPrefix(input, "goto "):
		m.gotoPageFromCmd(input)
	case strings.HasPrefix(input, "search "):
		term := strings.TrimSpace(input[len("search "):])
		if term == "" {
			m.statusMessage = "enter a term after search"
			return
		}
		m.searchTerm = term
		m.matches = findMatches(m.pages, term)
		if len(m.matches) == 0 {
			m.statusMessage = fmt.Sprintf("No matches for %s", term)
			return
		}
		m.matchIndex = 0
		m.currentPage = m.matches[0]
		m.viewport.GotoTop()
		m.refreshPage()
		m.mode = "SEARCH"
		m.statusMessage = fmt.Sprintf("Found %d matches", len(m.matches))
	case strings.HasPrefix(input, "export"):
		m.exportPage()
	default:
		m.statusMessage = fmt.Sprintf("Unknown command: %s", input)
	}
}

func (m *pdfModel) gotoPageFromCmd(input string) {
	fields := strings.Fields(input)
	if len(fields) < 2 {
		m.statusMessage = "usage: goto <page>"
		return
	}
	page, err := strconv.Atoi(fields[1])
	if err != nil || page < 1 || page > m.totalPages {
		m.statusMessage = "invalid page number"
		return
	}
	m.currentPage = page
	m.viewport.GotoTop()
	m.refreshPage()
	m.statusMessage = fmt.Sprintf("Moved to page %d", page)
}

func (m *pdfModel) exportPage() {
	if len(m.pages) == 0 || m.currentPage < 1 || m.currentPage > len(m.pages) {
		m.statusMessage = "No page available to export"
		return
	}
	file := fmt.Sprintf("export_page_%d.txt", m.currentPage)
	if err := os.WriteFile(file, []byte(m.pages[m.currentPage-1]), 0o644); err != nil {
		m.statusMessage = fmt.Sprintf("export failed: %v", err)
		return
	}
	m.statusMessage = fmt.Sprintf("Exported page %d to %s", m.currentPage, file)
}

func (m *pdfModel) handleNavigation(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyPgDown:
		m.nextPage()
		return m, nil
	case tea.KeyPgUp:
		m.prevPage()
		return m, nil
	}

	switch msg.String() {
	case "j", "down":
		m.viewport.LineDown(1)
	case "k", "up":
		m.viewport.LineUp(1)
	case "J", " ", "pgdn", "pgdown", "pagedown", "ctrl+f":
		m.nextPage()
	case "K", "pgup", "pageup", "repg", "ctrl+b":
		m.prevPage()
	case ":":
		m.commandMode = true
		m.showHelp = false
		m.mode = "COMMAND"
		m.commandBuffer = ""
		m.statusMessage = "Command mode - type help"
	case "/":
		m.commandMode = true
		m.showHelp = false
		m.mode = "COMMAND"
		m.commandBuffer = "search "
		m.statusMessage = "Search mode"
	case "?":
		m.showHelp = !m.showHelp
		if m.showHelp {
			m.mode = "HELP"
			m.statusMessage = "Help opened (Esc to close)"
		} else {
			m.mode = "NORMAL"
			m.statusMessage = "Help closed"
		}
	case "n":
		m.nextMatch()
	case "N":
		m.prevMatch()
	}
	return m, nil
}

func (m *pdfModel) nextPage() {
	if m.currentPage >= m.totalPages {
		m.statusMessage = "Already at last page"
		return
	}
	m.currentPage++
	m.viewport.GotoTop()
	m.refreshPage()
	m.statusMessage = fmt.Sprintf("Page %d/%d", m.currentPage, m.totalPages)
}

func (m *pdfModel) prevPage() {
	if m.currentPage <= 1 {
		m.statusMessage = "Already at first page"
		return
	}
	m.currentPage--
	m.viewport.GotoTop()
	m.refreshPage()
	m.statusMessage = fmt.Sprintf("Page %d/%d", m.currentPage, m.totalPages)
}

func (m *pdfModel) nextMatch() {
	if len(m.matches) == 0 {
		m.statusMessage = "No active search"
		return
	}
	m.matchIndex = (m.matchIndex + 1) % len(m.matches)
	m.currentPage = m.matches[m.matchIndex]
	m.viewport.GotoTop()
	m.refreshPage()
	m.statusMessage = fmt.Sprintf("Match %d/%d", m.matchIndex+1, len(m.matches))
}

func (m *pdfModel) prevMatch() {
	if len(m.matches) == 0 {
		m.statusMessage = "No active search"
		return
	}
	m.matchIndex = (m.matchIndex - 1 + len(m.matches)) % len(m.matches)
	m.currentPage = m.matches[m.matchIndex]
	m.viewport.GotoTop()
	m.refreshPage()
	m.statusMessage = fmt.Sprintf("Match %d/%d", m.matchIndex+1, len(m.matches))
}

func (m *pdfModel) refreshPage() {
	if len(m.pages) == 0 {
		m.viewport.SetContent("(no pages loaded)")
		return
	}
	if m.currentPage < 1 {
		m.currentPage = 1
	}
	if m.currentPage > len(m.pages) {
		m.currentPage = len(m.pages)
	}
	content := m.pages[m.currentPage-1]
	if m.searchTerm != "" {
		content = highlightText(content, m.searchTerm)
	}
	// Wrap content to viewport width so long lines get broken into
	// readable rows. Use a conservative max width to account for
	// viewport padding and borders.
	wrapWidth := m.viewport.Width - 4
	if wrapWidth < 20 {
		wrapWidth = m.viewport.Width
	}
	if wrapWidth > 0 {
		content = wrapText(content, wrapWidth)
	}
	m.viewport.SetContent(content)
}

// wrapText splits text into lines no longer than maxWidth. It preserves
// existing newlines and tries to break at spaces when possible; if a token
// has no spaces and exceeds maxWidth, it is hard-wrapped.
func wrapText(s string, maxWidth int) string {
	if s == "" || maxWidth <= 0 {
		return s
	}
	var out []string
	for _, line := range strings.Split(s, "\n") {
		rline := []rune(line)
		if len(rline) == 0 {
			out = append(out, "")
			continue
		}
		for len(rline) > 0 {
			if len(rline) <= maxWidth {
				out = append(out, string(rline))
				break
			}
			// look for last space within maxWidth
			cut := maxWidth
			found := -1
			for i := 0; i < maxWidth; i++ {
				if unicode.IsSpace(rline[i]) {
					found = i
				}
			}
			if found > 0 {
				// break at found (skip trailing spaces)
				part := strings.TrimRight(string(rline[:found]), " \t")
				out = append(out, part)
				// skip spaces after
				j := found
				for j < len(rline) && unicode.IsSpace(rline[j]) {
					j++
				}
				rline = rline[j:]
				continue
			}
			// no space found: hard wrap
			out = append(out, string(rline[:cut]))
			rline = rline[cut:]
		}
	}
	return strings.Join(out, "\n")
}

func (m pdfModel) View() string {
	headerPathMax := 40
	if m.viewport.Width > 0 {
		headerPathMax = maxInt(18, m.viewport.Width/3)
	}
	headerContent := fmt.Sprintf(" NvReader | %s | page %d/%d | mode %s ", shortenPath(m.path, headerPathMax), m.currentPage, m.totalPages, m.mode)
	if m.viewport.Width > 0 {
		headerContent = truncateWithEllipsis(headerContent, m.viewport.Width+2)
	}
	header := headerStyle.Render(headerContent)
	bodyContent := m.viewport.View()
	if m.showHelp {
		bodyContent = m.renderHelpBody()
	}
	body := viewportStyle.Render(bodyContent)
	statusParts := []string{fmt.Sprintf("matches %d", len(m.matches))}
	if m.statusMessage != "" {
		statusParts = append(statusParts, m.statusMessage)
	}
	statusPathMax := 60
	if m.viewport.Width > 0 {
		statusPathMax = maxInt(20, m.viewport.Width/3)
	}
	statusContent := fmt.Sprintf(" %s | %s ", shortenPath(m.path, statusPathMax), strings.Join(statusParts, " | "))
	if m.viewport.Width > 0 {
		statusContent = truncateWithEllipsis(statusContent, m.viewport.Width+2)
	}
	statusLine := statusStyle.Render(statusContent)
	commandLine := ""
	if m.commandMode {
		cmd := ":" + m.commandBuffer
		if m.viewport.Width > 0 {
			cmd = truncateWithEllipsis(cmd, m.viewport.Width+2)
		}
		commandLine = commandStyle.Render(cmd)
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, body, statusLine, commandLine)
}

func shortenPath(path string, max int) string {
	if len(path) <= max {
		return path
	}
	if max <= 2 {
		return "..."
	}
	return "..." + path[len(path)-max+3:]
}

func findMatches(pages []string, term string) []int {
	lowerTerm := strings.ToLower(term)
	matches := make([]int, 0)
	for idx, page := range pages {
		if strings.Contains(strings.ToLower(page), lowerTerm) {
			matches = append(matches, idx+1)
		}
	}
	return matches
}

func truncateWithEllipsis(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (m pdfModel) renderHelpBody() string {
	lines := []string{
		"HELP",
		"",
		"Navigation:",
		"  j / k          Scroll line down / up",
		"  J / K          Next / previous page",
		"  PgDn / PgUp    Next / previous page",
		"  n / N          Next / previous search match",
		"",
		"Command mode (:)",
		"  help           Open this panel",
		"  goto <n>       Go to page n",
		"  search <term>  Search term across pages",
		"  export         Export current page to txt",
		"",
		"Other:",
		"  /              Start search command",
		"  ?              Toggle help",
		"  Esc            Close help / cancel command",
		"  q              Quit",
	}
	content := strings.Join(lines, "\n")
	if m.viewport.Width > 0 {
		panelWidth := m.viewport.Width - 4
		if panelWidth < 30 {
			panelWidth = 30
		}
		return helpStyle.MaxWidth(panelWidth).Render(content)
	}
	return helpStyle.Render(content)
}
