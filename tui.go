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
It optionally injects dynamic highlights for active searches and invokes
the line-wrapping sub-routine to preserve readability margins.
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
	/*
	   Background pre-fetching buffer.
	   Triggering preloadAround ensures seamless reading transitions 
	   by mounting surrounding pages into the thread-safe map preemptively.
	*/
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

/*
wrapText applies word wrapping to ensure the extracted PDF paragraph fits seamlessly
within the boundaries of the terminal viewport.
It performs a hard wrap on maxWidth boundaries, and fully justifies the lines 
by inserting mathematically distributed padding spaces between words. 
The final line of any paragraph natively retains its standard left alignment.
*/
func wrapText(s string, maxWidth int) string {
	if s == "" || maxWidth <= 0 {
		return s
	}

	var out []string
	paragraphs := strings.Split(s, "\n")
	
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			out = append(out, "")
			continue
		}

		words := strings.Fields(p)
		var currentLine []string
		var currentLineLen int

		for _, word := range words {
			wordLen := len([]rune(word))
			
			if len(currentLine) > 0 && currentLineLen+len(currentLine)+wordLen > maxWidth {
				out = append(out, justifyLine(currentLine, currentLineLen, maxWidth))
				currentLine = []string{word}
				currentLineLen = wordLen
			} else {
				currentLine = append(currentLine, word)
				currentLineLen += wordLen
			}
		}

		if len(currentLine) > 0 {
			out = append(out, strings.Join(currentLine, " "))
		}
	}

	return strings.Join(out, "\n")
}

/*
justifyLine computes the exact gap frequency required to distribute excess 
blank columns smoothly across the line, producing a perfectly flush right margin.
*/
func justifyLine(words []string, wordsLen int, maxWidth int) string {
	if len(words) == 1 {
		return words[0]
	}

	totalSpaces := maxWidth - wordsLen
	gaps := len(words) - 1

	spacesPerGap := totalSpaces / gaps
	extraSpaces := totalSpaces % gaps

	var builder strings.Builder
	for i, word := range words {
		builder.WriteString(word)
		if i < gaps {
			spacesToApply := spacesPerGap
			if i < extraSpaces {
				spacesToApply++
			}
			builder.WriteString(strings.Repeat(" ", spacesToApply))
		}
	}

	return builder.String()
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
