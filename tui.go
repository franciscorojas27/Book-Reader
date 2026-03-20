/*
DEV LOG - TUI ARCHITECTURE & FLOW RENDERING
-------------------------------------------------------------------------------
The TUI (Terminal User Interface) manages the visual flow of extracted PDF data.

Key architectural decisions:
1. Async Initialization: The app starts instantly. If a path is provided, it 
   enters LOADING state. If NOT, it enters EMPTY state with a welcome screen.
2. Viewport State: Every frame is passed to m.viewport.Update(msg) to handle
   internal scrolling state and terminal resize event buffering.
3. Multi-File Support: Using :open <path> or Ctrl+P, the user can switch 
   documents without restarting. This resets all caches and search matches.
4. Semantic Flow Packaging: wrapText ensures natural paragraphs by joining 
   fragmented lines and splitting only at sentence boundaries (~10 lines).
5. Safety Exit: Exit requires Ctrl+Q or :q to prevent accidental closure.
-------------------------------------------------------------------------------
*/
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	helpStyle     = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#f4b8e4")).Padding(1, 4)
	loadingStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7fdbca"))
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555")).Bold(true)
	welcomeStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#8ad6ff"))
)

type pdfModel struct {
	path          string
	cache         *pageCache
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
	loading       bool
	err           error
}

func newPDFModel(path string) pdfModel {
	vp := viewport.New(80, 20)
	m := pdfModel{
		path:          path,
		viewport:      vp,
		currentPage:   1,
	}
	if path == "" {
		m.mode = "EMPTY"
		m.statusMessage = "Welcome! Press Ctrl+P to open a PDF or ? for help"
	} else {
		m.mode = "LOADING"
		m.statusMessage = "Loading PDF structure..."
		m.loading = true
	}
	return m
}

func (m pdfModel) Init() tea.Cmd {
	if m.path != "" {
		return loadPDF(m.path)
	}
	return nil
}

func (m pdfModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	if !m.loading && m.mode != "EMPTY" && m.mode != "ERROR" {
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	switch msg := msg.(type) {
	case pdfLoadedMsg:
		m.loading = false
		m.cache = msg
		m.totalPages = m.cache.total
		m.mode = "NORMAL"
		m.statusMessage = "Press : for commands, / to search"
		m.refreshPage()
		m.viewport.GotoTop()
		return m, nil

	case errorMsg:
		m.loading = false
		m.err = msg
		m.mode = "ERROR"
		return m, nil

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
		if !m.loading && m.cache != nil {
			m.refreshPage()
			m.viewport.GotoTop()
		}
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		// EXIT Global: Ctrl+Q or Ctrl+C
		if msg.Type == tea.KeyCtrlC || msg.String() == "ctrl+q" {
			return m, tea.Quit
		}

		// Loading/Error bypass
		if m.loading { return m, nil }

		// Help handling
		if m.showHelp {
			if msg.Type == tea.KeyEsc || msg.String() == "?" {
				m.showHelp = false
				if m.path == "" { m.mode = "EMPTY" } else { m.mode = "NORMAL" }
				return m, nil
			}
			return m, nil
		}

		// Command mode handling
		if m.commandMode {
			resModel, resCmd := m.handleCommandMode(msg)
			return resModel, tea.Batch(append(cmds, resCmd)...)
		}

		// OPEN Shortcut: Ctrl+P
		if msg.Type == tea.KeyCtrlP {
			m.commandMode = true
			m.mode = "COMMAND"
			m.commandBuffer = "open "
			m.statusMessage = "Enter PDF file path"
			return m, nil
		}

		if m.mode == "EMPTY" || m.err != nil {
			if msg.String() == ":" {
				m.commandMode = true
				m.mode = "COMMAND"
				m.commandBuffer = ""
				return m, nil
			}
			if msg.String() == "?" {
				m.showHelp = true
				m.mode = "HELP"
				return m, nil
			}
			return m, nil
		}

		resModel, resCmd := m.handleNavigation(msg)
		return resModel, tea.Batch(append(cmds, resCmd)...)
	}

	return m, tea.Batch(cmds...)
}

func (m pdfModel) handleCommandMode(msg tea.KeyMsg) (pdfModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.commandMode = false
		m.commandBuffer = ""
		if m.path == "" { m.mode = "EMPTY" } else { m.mode = "NORMAL" }
		m.statusMessage = "Command cancelled"
		return m, nil
	case tea.KeyEnter:
		m.commandMode = false
		input := strings.TrimSpace(m.commandBuffer)
		m.commandBuffer = ""
		if m.path == "" { m.mode = "EMPTY" } else { m.mode = "NORMAL" }
		return m.processCommand(input)
	case tea.KeyBackspace, tea.KeyDelete:
		if len(m.commandBuffer) > 0 {
			m.commandBuffer = m.commandBuffer[:len(m.commandBuffer)-1]
		}
		return m, nil
	default:
		if msg.Type == tea.KeyRunes {
			m.commandBuffer += string(msg.Runes)
		} else if msg.Type == tea.KeySpace {
			m.commandBuffer += " "
		}
		return m, nil
	}
}

func (m pdfModel) processCommand(input string) (pdfModel, tea.Cmd) {
	switch {
	case input == "":
		m.statusMessage = "No command entered"
	case input == "q", input == "quit", input == "exit":
		return m, tea.Quit
	case input == "tools", input == "help", input == "?", input == ":help", input == "commands":
		m.showHelp = true
		m.mode = "HELP"
		m.statusMessage = "Help opened (Esc to close)"
	case strings.HasPrefix(input, "open "):
		path := strings.TrimSpace(input[len("open "):])
		path = strings.Trim(path, "\"'") // Strip quotes if pasted with them
		
		// Aggressive sanitization: remove all non-printable characters 
		// (like BOMs or hidden control codes from terminal paste)
		var cleaned strings.Builder
		for _, r := range path {
			if r >= 32 && r != 127 { // Standard printable ASCII range
				cleaned.WriteRune(r)
			}
		}
		path = strings.TrimSpace(cleaned.String())
		path = filepath.Clean(path)

		if path == "" {
			m.statusMessage = "Usage: open <path>"
			return m, nil
		}
		// Reset state for new load
		m.path = path
		m.loading = true
		m.err = nil
		m.cache = nil
		m.totalPages = 0
		m.currentPage = 1
		m.matches = nil
		m.searchTerm = ""
		m.mode = "LOADING"
		m.statusMessage = "Opening " + path + "..."
		return m, loadPDF(m.path)
	case strings.HasPrefix(input, "goto "):
		if m.path == "" { m.statusMessage = "No PDF loaded"; return m, nil }
		m.gotoPageFromCmd(input)
	case strings.HasPrefix(input, "search "):
		if m.path == "" { m.statusMessage = "No PDF loaded"; return m, nil }
		term := strings.TrimSpace(input[len("search "):])
		if term == "" {
			m.statusMessage = "enter a term after search"
			return m, nil
		}
		m.searchTerm = term
		m.matches = m.findMatches(term)
		if len(m.matches) == 0 {
			m.statusMessage = fmt.Sprintf("No matches for %s", term)
			return m, nil
		}
		m.matchIndex = 0
		m.currentPage = m.matches[0]
		m.viewport.GotoTop()
		m.refreshPage()
		m.mode = "SEARCH"
		m.statusMessage = fmt.Sprintf("Found %d matches", len(m.matches))
	case strings.HasPrefix(input, "export"):
		if m.path == "" { m.statusMessage = "No PDF loaded"; return m, nil }
		m.exportPage()
	default:
		m.statusMessage = fmt.Sprintf("Unknown command: %s", input)
	}
	return m, nil
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
	if m.cache == nil || m.currentPage < 1 || m.currentPage > m.totalPages {
		m.statusMessage = "No page available to export"
		return
	}
	content := m.cache.get(m.currentPage)
	file := fmt.Sprintf("export_page_%d.txt", m.currentPage)
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		m.statusMessage = fmt.Sprintf("export failed: %v", err)
		return
	}
	m.statusMessage = fmt.Sprintf("Exported page %d to %s", m.currentPage, file)
}

func (m pdfModel) handleNavigation(msg tea.KeyMsg) (pdfModel, tea.Cmd) {
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
		if m.viewport.AtBottom() {
			m.nextPage()
		} else {
			m.viewport.LineDown(m.viewport.Height - 1)
		}
	case "K", "pgup", "pageup", "repg", "ctrl+b":
		if m.viewport.AtTop() {
			m.prevPage()
		} else {
			m.viewport.LineUp(m.viewport.Height - 1)
		}
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
	if m.cache == nil || m.totalPages == 0 {
		m.viewport.SetContent("(no pages loaded)")
		return
	}
	content := m.cache.get(m.currentPage)
	m.cache.preloadAround(m.currentPage, 3)

	if m.searchTerm != "" {
		content = highlightText(content, m.searchTerm)
	}
	wrapWidth := m.viewport.Width - 4
	if wrapWidth < 20 { wrapWidth = m.viewport.Width }
	if wrapWidth > 0 {
		content = wrapText(content, wrapWidth)
	}
	m.viewport.SetContent(content)
}

func wrapText(s string, maxWidth int) string {
	if s == "" || maxWidth <= 0 { return s }
	fields := strings.Fields(s)
	if len(fields) == 0 { return "" }

	var words []string
	for _, f := range fields {
		if len(words) > 0 && (f == "," || f == "." || f == ":" || f == ";" || f == "!" || f == "?") {
			words[len(words)-1] += f
		} else {
			words = append(words, f)
		}
	}

	var out strings.Builder
	var currentLine strings.Builder
	lineCount := 0

	flushCurrent := func() {
		if currentLine.Len() > 0 {
			out.WriteString(strings.TrimSpace(currentLine.String()))
			out.WriteByte('\n')
			currentLine.Reset()
			lineCount++
		}
	}

	for _, word := range words {
		wordLen := len([]rune(word))
		if currentLine.Len() > 0 && currentLine.Len()+1+wordLen > maxWidth {
			flushCurrent()
		}
		if currentLine.Len() > 0 { currentLine.WriteByte(' ') }
		currentLine.WriteString(word)

		if lineCount >= 10 && (strings.HasSuffix(word, ".") || strings.HasSuffix(word, "!") || strings.HasSuffix(word, "?")) {
			flushCurrent()
			out.WriteByte('\n')
			lineCount = 0
		}
	}
	if currentLine.Len() > 0 {
		out.WriteString(strings.TrimSpace(currentLine.String()))
	}
	return out.String()
}

func (m pdfModel) View() string {
	headerPathMax := 40
	if m.viewport.Width > 0 {
		headerPathMax = maxInt(18, m.viewport.Width/3)
	}
	currentPath := m.path
	if currentPath == "" { currentPath = "(none)" }
	headerContent := fmt.Sprintf(" NvReader | %s | page %d/%d | mode %s ", shortenPath(currentPath, headerPathMax), m.currentPage, m.totalPages, m.mode)
	if m.viewport.Width > 0 {
		headerContent = truncateWithEllipsis(headerContent, m.viewport.Width+2)
	}
	header := headerStyle.Render(headerContent)

	var bodyContent string
	switch {
	case m.err != nil:
		bodyContent = errorStyle.Render(fmt.Sprintf("\n  ERROR: %v\n\n  - Press Ctrl+P or :open to try another file\n  - Press Ctrl+Q to quit", m.err))
	case m.loading:
		bodyContent = loadingStyle.Render("\n  [⌛] Loading PDF structure...\n  Please wait a moment.")
	case m.mode == "EMPTY":
		bodyContent = welcomeStyle.Render("\n  Welcome to NvReader!\n\n  No PDF is currently open.\n\n  Commands:\n    Ctrl+P    Open a PDF file\n    :open     Open a PDF file\n    ?         Toggle Help\n    Ctrl+Q    Quit")
	default:
		bodyContent = m.viewport.View()
		if m.showHelp {
			bodyContent = m.renderHelpBody()
		}
	}
	
	// Stabilize height and width to prevent jitter
	bodyWidth := m.viewport.Width
	bodyHeight := m.viewport.Height
	if bodyWidth > 0 && bodyHeight > 0 {
		bodyContent = lipgloss.NewStyle().
			Width(bodyWidth).
			Height(bodyHeight).
			Render(bodyContent)
	}
	
	body := viewportStyle.Render(bodyContent)

	statusLine := statusStyle.Render(truncateWithEllipsis(fmt.Sprintf(" %s | %s ", shortenPath(currentPath, 60), m.statusMessage), m.viewport.Width+2))

	commandLine := ""
	if m.commandMode {
		cmd := ":" + m.commandBuffer
		if m.viewport.Width > 0 { cmd = truncateWithEllipsis(cmd, m.viewport.Width+2) }
		commandLine = commandStyle.Render(cmd)
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, body, statusLine, commandLine)
}

func shortenPath(path string, max int) string {
	if path == "" { return "" }
	if len(path) <= max { return path }
	if max <= 2 { return "..." }
	return "..." + path[len(path)-max+3:]
}

func (m *pdfModel) findMatches(term string) []int {
	if m.cache == nil { return nil }
	lowerTerm := strings.ToLower(term)
	var matches []int
	for i := 1; i <= m.totalPages; i++ {
		pageText := m.cache.get(i)
		if strings.Contains(strings.ToLower(pageText), lowerTerm) {
			matches = append(matches, i)
		}
	}
	return matches
}

func truncateWithEllipsis(s string, max int) string {
	if max <= 0 { return "" }
	runes := []rune(s)
	if len(runes) <= max { return s }
	if max <= 3 { return string(runes[:max]) }
	return string(runes[:max-3]) + "..."
}

func maxInt(a, b int) int {
	if a > b { return a }
	return b
}

func (m pdfModel) renderHelpBody() string {
	lines := []string{
		"HELP",
		"",
		"General Commands:",
		"  Ctrl+P / :open <path>  Open a NEW PDF file",
		"  Ctrl+Q / :q / :quit    Exit application",
		"  ? / help               Toggle this panel",
		"",
		"Navigation:",
		"  j / k          Scroll line down / up",
		"  J / K          Next / previous page",
		"  PgDn / PgUp    Next / previous page",
		"  n / N          Next / previous search match",
		"",
		"Advanced:",
		"  / <term>       Search term across pages",
		"  :goto <n>      Go to page n",
		"  :export        Export current page to txt",
		"  Esc            Cancel command / Close help",
	}
	content := strings.Join(lines, "\n")
	if m.viewport.Width > 0 {
		panelWidth := m.viewport.Width - 4
		if panelWidth < 30 { panelWidth = 30 }
		return helpStyle.MaxWidth(panelWidth).Render(content)
	}
	return helpStyle.Render(content)
}
