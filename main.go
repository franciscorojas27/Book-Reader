package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run . <path-to-pdf>")
		os.Exit(1)
	}

	path := os.Args[1]
	pages, err := loadPages(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening PDF: %v\n", err)
		os.Exit(1)
	}

	program := tea.NewProgram(newPDFModel(path, pages), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if err := program.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Runtime error: %v\n", err)
		os.Exit(1)
	}
}
