/*
DEV LOG - TUI ARCHITECTURE & FLOW RENDERING
-------------------------------------------------------------------------------
The TUI (Terminal User Interface) manages the visual flow of extracted PDF data.

Key architectural decisions:
1. Viewport State: Every frame is passed to m.viewport.Update(msg) to handle
   internal scrolling state and terminal resize event buffering.
2. Semantic Flow Packaging: To improve readability on varying terminal widths,
   wrapText reconstructions paragraphs by joining the fragmented PDF lines.
3. Smart Paragraphing: We insert a paragraph break (double newline) 
   approximately every 10 lines upon hitting a sentence-ending punctuation
   (., !, ?), ensuring the text is visually digestible.
4. Punctuation Glue: Commas and periods are forcibly attached to their 
   preceding word to prevent orphaned symbols at the start of new lines.
-------------------------------------------------------------------------------
*/
package main

import (
	"fmt"
	"os"
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
	helpStyle     = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#f4b8e4")).Padding(1, 2)
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
}

func newPDFModel(path string, cache *pageCache) pdfModel {
	vp := viewport.New(80, 20)
	model := pdfModel{
		path:          path,
		cache:         cache,
		viewport:      vp,
		currentPage:   1,
		totalPages:    cache.total,
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
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	// CRITICAL: Viewport must receive all messages.
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

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
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC || msg.String() == "q" {
			return m, tea.Quit
		}
		if m.showHelp {
			if msg.Type == tea.KeyEsc || msg.String() == "?" {
				m.showHelp = false
				m.mode = "NORMAL"
				m.statusMessage = "Help closed"
				return m, tea.Batch(cmds...)
			}
		}
		if m.commandMode {
			resModel, resCmd := m.handleCommandMode(msg)
			return resModel, tea.Batch(append(cmds, resCmd)...)
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
		m.matches = m.findMatches(term)
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
	if m.currentPage < 1 || m.currentPage > m.totalPages {
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

/*
nextMatch sequentially rotates the document cursor to the subsequent
searched target found dynamically across the cached memory.
*/
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

/*
refreshPage pulls the resolved textual data for the actively focused page
and synchronously mounts it into the terminal viewport bounds.
*/
func (m *pdfModel) refreshPage() {
	if m.totalPages == 0 {
		m.viewport.SetContent("(no pages loaded)")
		return
	}
	if m.currentPage < 1 {
		m.currentPage = 1
	}
	if m.currentPage > m.totalPages {
		m.currentPage = m.totalPages
	}
	content := m.cache.get(m.currentPage)
	
	m.cache.preloadAround(m.currentPage, 3)

	if m.searchTerm != "" {
		content = highlightText(content, m.searchTerm)
	}
	wrapWidth := m.viewport.Width - 4
	if wrapWidth < 20 {
		wrapWidth = m.viewport.Width
	}
	if wrapWidth > 0 {
		content = wrapText(content, wrapWidth)
	}
	m.viewport.SetContent(content)
}

func wrapText(s string, maxWidth int) string {
	if s == "" || maxWidth <= 0 {
		return s
	}

	// Step 1: Flatten everything into words. 
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}

	// Step 2: Glue orphaned punctuation to the previous word.
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
		
		// Normal wrap check.
		if currentLine.Len() > 0 && currentLine.Len()+1+wordLen > maxWidth {
			flushCurrent()
		}

		if currentLine.Len() > 0 {
			currentLine.WriteByte(' ')
		}
		currentLine.WriteString(word)

		// Semantic Paragraph Break check:
		// If we've committed ~10 lines and just finished a sentence, flush it 
		// and add a double newline to start a clean new paragraph.
		if lineCount >= 10 && (strings.HasSuffix(word, ".") || strings.HasSuffix(word, "!") || strings.HasSuffix(word, "?")) {
			flushCurrent()
			out.WriteByte('\n')
			lineCount = 0
		}
	}

	// Final flush.
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

/*
findMatches executes a lazy-loaded linear scan over all strictly indexed
document pages to isolate sub-string hits.
*/
func (m *pdfModel) findMatches(term string) []int {
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
