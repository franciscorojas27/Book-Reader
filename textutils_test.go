package main

import (
	"strings"
	"testing"
)

func TestIndentText(t *testing.T) {
	in := "line1\n\nline2"
	want := "    line1\n\n    line2"
	got := indentText(in)
	if got != want {
		t.Fatalf("indentText() = %q, want %q", got, want)
	}
}

func TestHighlightText(t *testing.T) {
	in := "Hello World hello"
	term := "hello"
	got := highlightText(in, term)
	plain := stripANSI(got)
	if !strings.Contains(strings.ToLower(plain), term) {
		t.Fatalf("highlightText result does not contain term %q: %q", term, plain)
	}
}
