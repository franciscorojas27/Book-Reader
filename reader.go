package main

/*
DEV LOG - PDF EXTRACTION ENGINE (Td/Tm Radar)
-------------------------------------------------------------------------------
The extraction engine solves the "missing spaces" problem in justified PDFs.

Key technical breakthroughs:
1. Stream Interception: Instead of using GetPlainText (which ignores kerning) 
   or GetTextByRow (which fragments strings), we use pdf.Interpret to 
   manually listen to the binary 'Contents' stream.
2. The Td/Tm Discovery: In justified PDFs, "to design" might be split 
   across lines. Standard extractors often merge the last word of one line 
   with the first word of the next ("todesign"). By intercepting matrix 
   movement (Td, TD, Tm), we detect vertical Y-axis jumps >1.0 and 
   forcibly inject a newline. 
3. This preserves word integrity without needing complex dictionary-based 
   heuristics or font-metric estimators.
-------------------------------------------------------------------------------
*/

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"sync"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dslipak/pdf"
)

type pdfLoadedMsg *pageCache
type errorMsg error

/*
pageCache holds lazily-loaded page text to guarantee the application 
starts instantly without fully mounting the PDF upfront into memory.
*/
type pageCache struct {
	mu     sync.Mutex
	reader *pdf.Reader
	total  int
	cache  map[int]string
}

func newPageCache(r *pdf.Reader) *pageCache {
	return &pageCache{
		reader: r,
		total:  r.NumPage(),
		cache:  make(map[int]string),
	}
}

/*
get returns the text layout string for page i (1-indexed).
It locks around the hashmap to concurrently serve pages.
*/
func (pc *pageCache) get(i int) string {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	if text, ok := pc.cache[i]; ok {
		return text
	}
	pg := pc.reader.Page(i)
	if pg.V.IsNull() {
		pc.cache[i] = ""
		return ""
	}
	txt, err := extractPageText(pg)
	if err != nil {
		txt = fmt.Sprintf("Error reading page %d: %v", i, err)
	}
	pc.cache[i] = txt
	return txt
}

/*
preloadAround automatically spins up headless goroutines 
to pre-mount adjacent pages while the user is reading.
*/
func (pc *pageCache) preloadAround(current, radius int) {
	for delta := 1; delta <= radius; delta++ {
		for _, candidate := range []int{current + delta, current - delta} {
			if candidate < 1 || candidate > pc.total {
				continue
			}
			pc.mu.Lock()
			_, loaded := pc.cache[candidate]
			pc.mu.Unlock()
			if !loaded {
				go func(idx int) {
					_ = pc.get(idx)
				}(candidate)
			}
		}
	}
}

/*
loadPages serves as the entry ingestion method. It builds the 
pointer maps but delays text processing entirely to the cache.
*/
func loadPages(path string) (*pageCache, error) {
	r, err := pdf.Open(path)
	if err != nil {
		return nil, err
	}
	return newPageCache(r), nil
}

func loadPDF(path string) tea.Cmd {
	return func() tea.Msg {
		cache, err := loadPages(path)
		if err != nil {
			return errorMsg(err)
		}
		return pdfLoadedMsg(cache)
	}
}

/*
extractPageText is the core text alignment engine.
It intercepts the raw PDF binary array (Contents) and builds the string stream manually. 
This bypasses dslipak/pdf's unreliable threshold gaps. 
Instead of calculating font metric ranges, this tracks native line-breaks (Td, Tm) 
to inject hard splits when words wrap naturally down the page.
*/
func extractPageText(pg pdf.Page) (string, error) {
	strm := pg.V.Key("Contents")

	fonts := make(map[string]*pdf.Font)
	for _, fontName := range pg.Fonts() {
		f := pg.Font(fontName)
		fonts[fontName] = &f
	}

	var b strings.Builder
	var enc pdf.TextEncoding

	showText := func(s string) {
		if enc != nil {
			for _, ch := range enc.Decode(s) {
				b.WriteRune(ch)
			}
		} else {
			b.WriteString(s)
		}
	}

	var err error
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("pdf interpretation panic: %v", r)
		}
	}()

	var lastY float64 = -9999
	isFirst := true

	pdf.Interpret(strm, func(stk *pdf.Stack, op string) {
		n := stk.Len()
		args := make([]pdf.Value, n)
		for i := n - 1; i >= 0; i-- {
			args[i] = stk.Pop()
		}

		switch op {
		case "T*":
			if !isFirst {
				b.WriteByte('\n')
			}
			isFirst = false
		case "Tf":
			if len(args) == 2 {
				f := args[0].Name()
				if font, ok := fonts[f]; ok {
					enc = font.Encoder()
				} else {
					enc = nil
				}
			}
		case "\"":
			if len(args) == 3 {
				if !isFirst {
					b.WriteByte('\n')
				}
				showText(args[2].RawString())
				isFirst = false
			}
		case "'":
			if len(args) == 1 {
				if !isFirst {
					b.WriteByte('\n')
				}
				showText(args[0].RawString())
				isFirst = false
			}
		case "Td", "TD":
			/*
			   Td/TD: Relative position translation operations.
			   By calculating if the vertical shift (ty) is strictly larger than 1.0, 
			   we intercept implicit line breaks and forcibly split the bounding text.
			*/
			if len(args) == 2 {
				ty := args[1].Float64()
				if !isFirst && math.Abs(ty) > 1.0 {
					b.WriteByte('\n')
				}
			}
			isFirst = false
		case "Tm":
			/*
			   Tm: Absolute matrix translation overrides.
			   Acts exactly as Td, but locks onto absolute offsets. Significant shifting
			   likewise implies a physical line flush.
			*/
			if len(args) == 6 {
				ty := args[5].Float64()
				if !isFirst && lastY != -9999 && math.Abs(ty-lastY) > 1.0 {
					b.WriteByte('\n')
				}
				lastY = ty
			}
			isFirst = false
		case "Tj":
			if len(args) == 1 {
				showText(args[0].RawString())
			}
			isFirst = false
		case "TJ":
			if len(args) > 0 {
				v := args[0]
				for i := 0; i < v.Len(); i++ {
					x := v.Index(i)
					if x.Kind() == pdf.String {
						showText(x.RawString())
					}
				}
			}
			isFirst = false
		}
	})

	if err != nil {
		return "", err
	}

	text := b.String()
	text = sanitizeExtractedText(text)

	/*
	   NLP Post-Processing
	   Normalization focuses on punctuation and redundant spacing. 
	*/
	reLetterNum := regexp.MustCompile(`([a-zA-Z])([0-9])`)
	text = reLetterNum.ReplaceAllString(text, "$1 $2")
	
	reNumLetter := regexp.MustCompile(`([0-9])([a-zA-Z])`)
	text = reNumLetter.ReplaceAllString(text, "$1 $2")

	text = simpleFixSpaces(text)

	return text, nil
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

/*
simpleFixSpaces applies deterministic regex rules (NLP) to clean up 
remnant PDF extraction artifacts.
*/
func simpleFixSpaces(s string) string {
	if strings.TrimSpace(s) == "" {
		return s
	}

	// Collapse orphaned letters (T a l e n t -> Talent)
	reSpaced := regexp.MustCompile(`(?i)(?:\b[A-Za-z]\s+){2,}[A-Za-z]\b`)
	s = reSpaced.ReplaceAllStringFunc(s, func(m string) string {
		return strings.ReplaceAll(m, " ", "")
	})

	// CRITICAL FIX: Eliminate ALL whitespace (including newlines) BEFORE punctuation.
	reBefore := regexp.MustCompile(`(?m)\s+([,.;:!?])`)
	s = reBefore.ReplaceAllString(s, "$1")

	// Ensure one space after punctuation
	reAfter := regexp.MustCompile(`([,.;:!?])(\S)`)
	s = reAfter.ReplaceAllString(s, "$1 $2")

	// Collapse multiple spaces to one
	reMulti := regexp.MustCompile(`\s{2,}`)
	s = reMulti.ReplaceAllString(s, " ")

	return strings.TrimSpace(s)
}
