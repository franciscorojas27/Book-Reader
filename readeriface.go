package main

import (
	"strings"

	"github.com/dslipak/pdf"
)

// PageGetter provides the minimal method we need from a PDF page.
type PageGetter interface {
	GetPlainText(map[string]*pdf.Font) (string, error)
}

// PageReader abstracts the pdf.Reader so we can unit test search logic.
type PageReader interface {
	NumPage() int
	Page(int) PageGetter
}

// pdfAdapter adapts *pdf.Reader to PageReader.
type pdfAdapter struct {
	r *pdf.Reader
}

func (p *pdfAdapter) NumPage() int {
	return p.r.NumPage()
}

func (p *pdfAdapter) Page(i int) PageGetter {
	return p.r.Page(i)
}

// newPDFAdapter creates an adapter from *pdf.Reader.
func newPDFAdapter(r *pdf.Reader) PageReader {
	return &pdfAdapter{r: r}
}

func searchAll(reader PageReader, term string) []int {
	if term == "" {
		return nil
	}
	lowerTerm := strings.ToLower(term)
	matches := make([]int, 0, reader.NumPage())
	for i := 1; i <= reader.NumPage(); i++ {
		text, err := reader.Page(i).GetPlainText(nil)
		if err != nil {
			continue
		}
		if strings.Contains(strings.ToLower(text), lowerTerm) {
			matches = append(matches, i)
		}
	}
	return matches
}
