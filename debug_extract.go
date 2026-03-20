//go:build ignore

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/dslipak/pdf"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run debug_extract.go <pdf-file> [page]")
		os.Exit(1)
	}
	path := os.Args[1]
	pageNum := 10 // default page
	if len(os.Args) >= 3 {
		fmt.Sscanf(os.Args[2], "%d", &pageNum)
	}

	r, err := pdf.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	pg := r.Page(pageNum)
	if pg.V.IsNull() {
		fmt.Println("Page is null")
		os.Exit(1)
	}

	// 1. GetPlainText
	fmt.Println("========== GetPlainText ==========")
	plainText, plainErr := pg.GetPlainText(nil)
	if plainErr != nil {
		fmt.Printf("Error: %v\n", plainErr)
	} else {
		lines := strings.Split(plainText, "\n")
		fmt.Printf("Lines: %d\n", len(lines))
		for i, line := range lines {
			if i > 30 {
				fmt.Println("... (truncated)")
				break
			}
			fmt.Printf("  L%d: %q\n", i, line)
		}
	}

	// 2. GetTextByRow
	fmt.Println("\n========== GetTextByRow ==========")
	rows, rowsErr := pg.GetTextByRow()
	if rowsErr != nil {
		fmt.Printf("Error: %v\n", rowsErr)
	} else {
		fmt.Printf("Rows: %d\n", len(rows))
		for ri, row := range rows {
			if ri > 20 {
				fmt.Println("... (truncated)")
				break
			}
			fmt.Printf("  Row %d (Y=%d): %d tokens\n", ri, row.Position, len(row.Content))
			for ti, tok := range row.Content {
				if ti > 15 {
					fmt.Printf("    ... (%d more tokens)\n", len(row.Content)-ti)
					break
				}
				fmt.Printf("    T%d: S=%q X=%.1f Y=%.1f W=%.1f FontSize=%.1f\n",
					ti, tok.S, tok.X, tok.Y, tok.W, tok.FontSize)
			}
		}
	}

	// 3. Content() - first 100 text items
	fmt.Println("\n========== Content().Text ==========")
	content := pg.Content()
	fmt.Printf("Text items: %d\n", len(content.Text))
	for i, t := range content.Text {
		if i > 80 {
			fmt.Println("... (truncated)")
			break
		}
		fmt.Printf("  I%d: S=%q X=%.1f Y=%.1f W=%.1f FontSize=%.1f Font=%q\n",
			i, t.S, t.X, t.Y, t.W, t.FontSize, t.Font)
	}
}
