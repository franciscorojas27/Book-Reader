package main

import (
	"errors"
	"testing"

	"github.com/dslipak/pdf"
)

type fakePage struct {
	txt string
}

func (f *fakePage) GetPlainText(_ map[string]*pdf.Font) (string, error) {
	if f.txt == "" {
		return "", errors.New("no text")
	}
	return f.txt, nil
}

type fakeReader struct {
	pages []PageGetter
}

func (f *fakeReader) NumPage() int          { return len(f.pages) }
func (f *fakeReader) Page(i int) PageGetter { return f.pages[i-1] }

func TestSearchAll(t *testing.T) {
	fr := &fakeReader{pages: []PageGetter{
		&fakePage{txt: "alpha beta"},
		&fakePage{txt: "gamma alpha"},
		&fakePage{txt: "nope"},
	}}
	got := searchAll(fr, "alpha")
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("searchAll returned %v, want [1 2]", got)
	}
}
