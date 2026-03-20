package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	path := ""
	if len(os.Args) >= 2 {
		path = os.Args[1]
	}
	program := tea.NewProgram(newPDFModel(path), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if err := program.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Runtime error: %v\n", err)
		os.Exit(1)
	}
}
