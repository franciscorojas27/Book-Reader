package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var highlightStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffffff")).Background(lipgloss.Color("#ff5f87"))

// highlightText highlights every instance of term inside t so that the terminal
// renderer can emphasize matches.
func highlightText(t, term string) string {
	if term == "" {
		return t
	}
	lowerTerm := strings.ToLower(term)
	lowerText := strings.ToLower(t)
	builder := strings.Builder{}
	start := 0
	for {
		idx := strings.Index(lowerText[start:], lowerTerm)
		if idx == -1 {
			builder.WriteString(t[start:])
			break
		}
		matchStart := start + idx
		builder.WriteString(t[start:matchStart])
		matchEnd := matchStart + len(term)
		builder.WriteString(highlightStyle.Render(t[matchStart:matchEnd]))
		start = matchEnd
	}
	return builder.String()
}

// indentText prefixes non-empty lines with four spaces to align them with
// nested content in the UI.
func indentText(input string) string {
	if input == "" {
		return ""
	}
	lines := strings.Split(input, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = "    " + line
		}
	}
	return strings.Join(lines, "\n")
}

// stripANSI removes ANSI escape sequences from a string.
func stripANSI(s string) string {
	// simple newline-preserving replacement; full regex for  CSI sequences
	out := ""
	inEsc := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if (c >= '@' && c <= '~') || c == '[' {
				inEsc = false
			}
			continue
		}
		out += string(c)
	}
	return out
}
