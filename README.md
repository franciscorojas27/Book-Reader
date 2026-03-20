# NvReader - Professional TUI PDF Reader

A high-performance Terminal User Interface (TUI) for reading PDF documents with a focus on typography, readability, and speed.

## ✨ Key Features

- **🚀 Instant Startup (Async Loading)**: Launches immediately using background ingestion. No more waiting for large PDFs to parse.
- **📡 Td/Tm Radar Extraction**: Uses a low-level PDF stream interpreter to accurately detect line breaks and preserve word integrity (e.g., compound words like `object-oriented`).
- **📖 Semantic Paragraphing**: Automatically reconstructs paragraphs from fragmented PDF lines for a natural reading flow.
- **🧲 Punctuation Glue**: Anchors symbols (`, . ! ?`) to their preceding words to prevent orphaned wraps.
- **🔍 Global Search**: Lightning-fast cross-page search with dynamic highlighting.
- **🌗 Multi-File Support**: Open or switch between PDF files within the app using `:open` or `Ctrl+P`.

## ⌨️ Shortcuts & Commands

### Navigation
| Key | Action |
| --- | --- |
| `j` / `k` | Scroll line down / up |
| `J` / `K` | Next / Previous PAGE |
| `PgDn` / `PgUp` | Next / Previous PAGE |
| `/` | Enter SEARCH mode |
| `n` / `N` | Next / Previous search match |
| `?` | Toggle HELP panel |

### Commands (Press `:`)
| Command | Action |
| --- | --- |
| `:open <path>` | Open a NEW PDF document |
| `:goto <n>` | Jump to page `n` |
| `:export` | Save the current page as a `.txt` file |
| `:q` / `:quit` | Exit the application |

### Global
| Key | Action |
| --- | --- |
| `Ctrl+P` | Quick-Open a new file |
| `Ctrl+Q` | Secure Exit |

## 🛠️ Installation & Usage

Requires [Go](https://golang.org/) 1.18+.

```bash
# Clone and run
go run . "path/to/your/document.pdf"

# Run without path to start in the Welcome Screen
go run .
```

## 🏗️ Technical Architecture

NvReader is built using:
- **[Bubbletea](https://github.com/charmbracelet/bubbletea)**: For the TUI event loop and concurrency.
- **[Lipgloss](https://github.com/charmbracelet/lipgloss)**: For professional-grade terminal styling.
- **[dslipak/pdf](https://github.com/dslipak/pdf)**: For low-level PDF stream access.
- **Custom Post-Processor**: An NLP-lite engine that cleans up extraction artifacts while preserving semantic meaning.

---
*Created with focus on visual excellence and developer experience.*