package main

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/dslipak/pdf"
)

func loadPages(path string) ([]string, error) {
	r, err := pdf.Open(path)
	if err != nil {
		return nil, err
	}
	total := r.NumPage()
	pages := make([]string, total)
	for i := 1; i <= total; i++ {
		pg := r.Page(i)
		if pg.V.IsNull() {
			pages[i-1] = ""
			continue
		}
		txt, err := extractPageText(pg)
		if err != nil {
			pages[i-1] = fmt.Sprintf("Error reading page %d: %v", i, err)
			continue
		}
		pages[i-1] = txt
	}
	return pages, nil
}

func extractPageText(pg pdf.Page) (string, error) {
	plainText, plainErr := pg.GetPlainText(nil)
	plainText = sanitizeExtractedText(plainText)
	// If plain extracted text is available, prefer it — it's usually
	// the cleanest representation and avoids fragile spacing heuristics.
	if strings.TrimSpace(plainText) != "" {
		// If plainText contains multiple short lines, it's probably well-formed
		// and we can return it directly. If it's a single very long line
		// (common with some PDFs), prefer row/content extraction instead.
		nl := strings.Count(plainText, "\n")
		if nl >= 1 {
			lines := strings.Split(plainText, "\n")
			total := 0
			for _, l := range lines {
				total += len(strings.TrimSpace(l))
			}
			avg := 0
			if len(lines) > 0 {
				avg = total / len(lines)
			}
			if avg < 200 {
				return plainText, nil
			}
		}
		// otherwise fall through and choose best among row/content
	}

	rowText := ""
	rows, rowsErr := pg.GetTextByRow()
	if rowsErr == nil {
		rowText = sanitizeExtractedText(rowsToText(rows))
	}

	contentText := sanitizeExtractedText(contentToText(pg))

	best := chooseBestPageText(plainText, rowText, contentText)
	// Convert page to a compact block: remove intra-token spaces so the
	// text is a continuous block, but ensure punctuation (.,;:!?) is
	// followed by a single space when followed by a letter/number.
	best = blockNormalize(best)
	if strings.TrimSpace(best) != "" {
		return best, nil
	}
	if plainErr != nil {
		return "", plainErr
	}
	if rowsErr != nil {
		return "", rowsErr
	}
	return "", nil
}

func rowsToText(rows pdf.Rows) string {
	var b strings.Builder
	for i, row := range rows {
		for _, word := range row.Content {
			// Do not insert spaces based on gap heuristics; simply
			// concatenate tokens. We'll apply simple punctuation-based
			// spacing later to ensure commas/periods are followed by a space.
			chunk := strings.TrimSpace(word.S)
			if chunk == "" {
				continue
			}
			b.WriteString(chunk)
		}
		if i < len(rows)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func chooseBestPageText(options ...string) string {
	best := ""
	bestScore := -1 << 30
	for _, option := range options {
		score := pageTextScore(option)
		if score > bestScore {
			best = option
			bestScore = score
		}
	}
	return best
}

func pageTextScore(s string) int {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return -1 << 29
	}
	words := strings.Fields(trimmed)
	wordCount := len(words)
	if wordCount == 0 {
		return -1 << 29
	}

	runeCount := len([]rune(trimmed))
	newlines := strings.Count(trimmed, "\n")
	replacements := strings.Count(trimmed, "\uFFFD")

	singleLetter := 0
	for _, w := range words {
		r := []rune(w)
		if len(r) == 1 && unicode.IsLetter(r[0]) {
			singleLetter++
		}
	}

	// Penalize heavily broken outputs that are mostly one-letter tokens.
	penalty := 0
	if wordCount >= 20 && singleLetter*100/wordCount >= 35 {
		penalty = 250
	}

	return runeCount + (wordCount * 3) + (newlines * 5) - (replacements * 40) - penalty
}

func sanitizeExtractedText(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\u0000' {
			continue
		}
		if unicode.IsControl(r) && r != '\n' && r != '\t' && r != '\r' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// collapseSpacedLetters joins sequences of single-letter tokens that likely
// represent a word split into separate glyphs (e.g., "F o r").
func collapseSpacedLetters(text string) string {
	if text == "" {
		return ""
	}
	var b strings.Builder
	lines := strings.Split(text, "\n")
	for li, line := range lines {
		if li > 0 {
			b.WriteByte('\n')
		}
		tokens := strings.Fields(line)
		if len(tokens) == 0 {
			continue
		}
		merged := make([]string, 0, len(tokens))
		for i := 0; i < len(tokens); {
			// Only aggressively merge runs of single-letter tokens
			// (e.g., "F o r"). This avoids joining short real words.
			if tokenIsLetters(tokens[i]) && len([]rune(tokens[i])) == 1 {
				j := i
				var runes []rune
				for j < len(tokens) {
					r := []rune(tokens[j])
					if len(r) == 1 && tokenIsLetters(tokens[j]) {
						runes = append(runes, r[0])
						j++
						continue
					}
					break
				}
				if j-i >= 3 {
					merged = append(merged, string(runes))
					i = j
					continue
				}
			}
			merged = append(merged, tokens[i])
			i++
		}
		for ti, tok := range merged {
			if ti > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(tok)
		}
	}
	return b.String()
}

func tokenIsLetters(token string) bool {
	if token == "" {
		return false
	}
	for _, r := range token {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

// simpleFixSpaces applies a small set of deterministic regex rules to
// (1) collapse obvious glyph-split runs like "F o r m a n y" -> "Formany",
// (2) remove spaces before punctuation, and
// (3) ensure exactly one space after punctuation.
func simpleFixSpaces(s string) string {
	if strings.TrimSpace(s) == "" {
		return s
	}
	// Collapse runs of single-letter tokens: (?:L\s+){3,}L
	reRun := regexp.MustCompile(`(?i)(?:([A-Za-z])\s+){3,}([A-Za-z])`)
	// Use ReplaceAllStringFunc to remove spaces inside each matched run.
	s = reRun.ReplaceAllStringFunc(s, func(m string) string {
		// remove all spaces inside the match
		return strings.ReplaceAll(m, " ", "")
	})

	// Remove spaces before punctuation
	reBefore := regexp.MustCompile(`\s+([,.;:!?])`)
	s = reBefore.ReplaceAllString(s, "$1")

	// Ensure one space after punctuation if it's immediately followed by a letter/number
	reAfter := regexp.MustCompile(`([,.;:!?])(\S)`)
	s = reAfter.ReplaceAllString(s, "$1 $2")

	// Collapse multiple spaces to one
	reMulti := regexp.MustCompile(`\s{2,}`)
	s = reMulti.ReplaceAllString(s, " ")

	// Trim spaces at line ends and rebuild lines
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	return strings.Join(lines, "\n")
}

// blockNormalize collapses spaces within lines (concatenates tokens)
// and then ensures punctuation is followed by a single space when
// immediately followed by a letter or digit. Newlines are preserved.
func blockNormalize(s string) string {
	if strings.TrimSpace(s) == "" {
		return s
	}
	reAfter := regexp.MustCompile(`([,.;:!?])([\p{L}\p{N}])`)
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		// remove spaces and tabs inside the line
		l := strings.ReplaceAll(line, " ", "")
		l = strings.ReplaceAll(l, "\t", "")
		// ensure one space after punctuation when followed by letter/number
		l = reAfter.ReplaceAllString(l, "$1 $2")
		lines[i] = strings.TrimSpace(l)
	}
	return strings.Join(lines, "\n")
}

// gapNeedsSpace decides whether a horizontal gap between two text chunks
// should be treated as a word space. It uses token lengths and average
// character widths to avoid inserting spaces between individual letters
// that PDF text extraction sometimes emits as separate tokens.
func gapNeedsSpace(prevS string, prevW, currW, gap float64, currS string) bool {
	// Simplified spacing heuristic: use a fixed absolute threshold.
	// Avoids complex width-based heuristics that can introduce spurious
	// spaces between glyph fragments. Treat as a word-space only when the
	// horizontal gap is clearly large.
	const spaceThreshold = 2.2
	return gap > spaceThreshold
}

func contentToText(pg pdf.Page) string {
	content := pg.Content()
	if len(content.Text) == 0 {
		return ""
	}

	texts := make([]pdf.Text, len(content.Text))
	copy(texts, content.Text)
	sort.Slice(texts, func(i, j int) bool {
		if math.Abs(texts[i].Y-texts[j].Y) > 0.5 {
			return texts[i].Y > texts[j].Y
		}
		return texts[i].X < texts[j].X
	})

	var b strings.Builder
	lastY := texts[0].Y
	// write first token if any
	firstChunk := strings.TrimSpace(texts[0].S)
	if firstChunk != "" {
		b.WriteString(firstChunk)
	}

	for idx := 1; idx < len(texts); idx++ {
		txt := texts[idx]
		if math.Abs(lastY-txt.Y) > 0.5 {
			b.WriteByte('\n')
			// new line: write token directly
			chunk := strings.TrimSpace(txt.S)
			if chunk != "" {
				b.WriteString(chunk)
			}
			// no gap-based spacing; nothing else to track
		} else {
			// Do not insert spaces based on gap heuristics; just append.
			chunk := strings.TrimSpace(txt.S)
			if chunk != "" {
				b.WriteString(chunk)
			}
		}
		lastY = txt.Y
	}

	return b.String()
}
