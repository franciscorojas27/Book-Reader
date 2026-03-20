package main

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"sync"
	"unicode"

	"github.com/dslipak/pdf"
)

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

	pdf.Interpret(strm, func(stk *pdf.Stack, op string) {
		n := stk.Len()
		args := make([]pdf.Value, n)
		for i := n - 1; i >= 0; i-- {
			args[i] = stk.Pop()
		}

		switch op {
		case "T*":
			b.WriteByte('\n')
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
				b.WriteByte('\n')
				showText(args[2].RawString())
			}
		case "'":
			if len(args) == 1 {
				b.WriteByte('\n')
				showText(args[0].RawString())
			}
		case "Td", "TD":
			/*
			   Td/TD: Relative position translation operations.
			   By calculating if the vertical shift (ty) is strictly larger than 1.0, 
			   we intercept implicit line breaks and forcibly split the bounding text.
			*/
			if len(args) == 2 {
				ty := args[1].Float64()
				if math.Abs(ty) > 1.0 {
					b.WriteByte('\n')
				}
			}
		case "Tm":
			/*
			   Tm: Absolute matrix translation overrides.
			   Acts exactly as Td, but locks onto absolute offsets. Significant shifting
			   likewise implies a physical line flush.
			*/
			if len(args) == 6 {
				ty := args[5].Float64()
				if lastY != -9999 && math.Abs(ty-lastY) > 1.0 {
					b.WriteByte('\n')
				}
				lastY = ty
			}
		case "Tj":
			if len(args) == 1 {
				showText(args[0].RawString())
			}
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
		}
	})

	if err != nil {
		return "", err
	}

	text := b.String()
	text = sanitizeExtractedText(text)

	/*
	   NLP Post-Processing
	   Despite tracking direct physical coordinates, some PDFs encode native
	   missing spaces in formatting elements missing tracking boundaries.
	   Conservatively slicing boundaries like numbers and hyphenations preserves
	   the aesthetic flow natively.
	*/
	reHyphen := regexp.MustCompile(`([a-z]+)-\n([a-z]+)`)
	text = reHyphen.ReplaceAllString(text, "$1$2\n")

	reCamel := regexp.MustCompile(`([a-z])([A-Z])`)
	text = reCamel.ReplaceAllString(text, "$1 $2")

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



// simpleFixSpaces applies a small set of deterministic regex rules to clean up
// common PDF extraction artifacts.
func simpleFixSpaces(s string) string {
	if strings.TrimSpace(s) == "" {
		return s
	}

	// Collapse runs of 3 or more single letters (e.g. "T a l e n t").
	// Using 3+ letters is much safer to avoid squashing valid single-letter words.
	reSpaced := regexp.MustCompile(`(?i)(?:\b[A-Za-z]\s+){2,}[A-Za-z]\b`)
	s = reSpaced.ReplaceAllStringFunc(s, func(m string) string {
		return strings.ReplaceAll(m, " ", "")
	})

	// Remove spaces before punctuation
	reBefore := regexp.MustCompile(`\s+([,.;:!?])`)
	s = reBefore.ReplaceAllString(s, "$1")

	// Ensure one space after punctuation if immediately followed by a letter/number
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


