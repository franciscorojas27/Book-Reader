package main

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dslipak/pdf"
	"github.com/eiannone/keyboard"
	"github.com/fatih/color"
)

var serverStatusMessages = []string{
	"Server response: 500 Internal Server Error - reconnecting...",
	"Server response: 503 Service Unavailable - retrying in 3s...",
	"Server response: 400 Bad Request - validating headers...",
	"Server response: 502 Bad Gateway - requesting new session...",
	"Server response: 504 Gateway Timeout - verifying connection...",
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

func readPage(r *pdf.Reader, page chan int, text chan string) {
	for p := range page {
		pg := r.Page(p)
		t, err := pg.GetPlainText(nil)
		if err != nil {
			text <- color.New(color.FgRed).Sprintf("Error al leer la página %d: %v", p, err)
			continue
		}
		text <- t
	}
}

func writePageOnConsole(text chan string, currentPage chan int) {
	var currentPageNumber int
	for {
		select {
		case t := <-text:
			fmt.Print("\033[H\033[2J")
			color.New(color.FgRed).Println(serverStatusMessages[rand.Intn(len(serverStatusMessages))])
			color.New(color.FgMagenta).Println("console > activity detected")
			color.New(color.FgCyan).Println("--------------------------------------")
			if strings.Contains(t, "Error al leer la página") {
				color.New(color.FgRed).Println(t)
			} else {
				color.New(color.FgGreen).Println(indentText(t))
			}
			color.New(color.FgYellow).Printf("\nEstás en la página: %d\n", currentPageNumber)
		case p := <-currentPage:
			currentPageNumber = p
		}
	}
}

func indentText(t string) string {
	lines := strings.Split(t, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			lines[i] = ""
		} else {
			lines[i] = "    " + line
		}
	}
	return strings.Join(lines, "\n")
}

func main() {
	text := make(chan string)
	pageNumber := make(chan int)
	currentPageChan := make(chan int)

	go writePageOnConsole(text, currentPageChan)

	if len(os.Args) < 2 {
		fmt.Println("Uso: go run . philosophy.pdf")
		return
	}

	path := os.Args[1]
	r, err := pdf.Open(path)
	if err != nil {
		color.New(color.FgRed).Printf("Error: %v\n", err)
		return
	}

	go readPage(r, pageNumber, text)

	if err := keyboard.Open(); err != nil {
		color.New(color.FgRed).Printf("Error al inicializar el teclado: %v\n", err)
		return
	}
	defer keyboard.Close()

	currentPage := 1
	pageNumber <- currentPage
	currentPageChan <- currentPage

	color.New(color.FgMagenta).Printf("Seleccione una página para leer (1 - %d):\n", r.NumPage())

	for {
		key, _, err := keyboard.GetKey()
		if err != nil {
			color.New(color.FgRed).Printf("Error: %v\n", err)
			continue
		}

		switch key {
		case 'j':
			if currentPage < r.NumPage() {
				currentPage++
				pageNumber <- currentPage
				currentPageChan <- currentPage
			}
		case 'k':
			if currentPage > 1 {
				currentPage--
				pageNumber <- currentPage
				currentPageChan <- currentPage
			}
		case 'q':
			close(pageNumber)
			close(text)
			close(currentPageChan)
			os.Exit(0)
		case 'g':
			fmt.Print("\nIngrese el número de página: ")
			var input string
			fmt.Scanln(&input)
			page, err := strconv.Atoi(input)
			if err != nil || page < 1 || page > r.NumPage() {
				color.New(color.FgRed).Println("Número de página no válido.")
			} else {
				currentPage = page
				pageNumber <- currentPage
				currentPageChan <- currentPage
			}
		}
	}
}
