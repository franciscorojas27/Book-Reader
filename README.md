# NvReader

NvReader is a terminal PDF reader built with Bubble Tea. It keeps page boundaries strict (one rendered page at a time), supports Vim-like navigation, and includes a command palette and help panel.

## Requirements

- Go 1.25+

## Run

```bash
go run . "path/to/file.pdf"
```

Windows example:

```bash
go run . "C:\Users\you\Documents\book.pdf"
```

## Navigation (Detailed)

NvReader has two levels of navigation:

1. In-page scrolling (moves inside the current page text).
2. Page navigation (changes page index, for example 14/210 -> 15/210).

### In-page scrolling

- `j` or `Down`: move one line down in current page
- `k` or `Up`: move one line up in current page

### Page navigation

- `J` or `Space`: next page
- `K`: previous page
- `PgDn` / `Av Pag`: next page
- `PgUp` / `Re Pag`: previous page
- `Ctrl+F`: next page
- `Ctrl+B`: previous page

Page navigation always keeps page boundaries. The app does not merge multiple pages into one scrolling buffer during normal reading.

## Command Mode

Press `:` to open command mode and type one of these commands:

- `help` (aliases: `commands`, `tools`, `?`, `:help`): open help panel
- `goto <n>`: jump to page `n`
- `search <term>`: search term across all pages
- `export`: export current page to `export_page_<n>.txt`

### Search flow

1. Press `/` (prefills command with `search `).
2. Type term and press `Enter`.
3. Use `n` and `N` to jump between matches.

## Quick keys

- `?`: toggle help panel
- `Esc`: close help panel or cancel command mode
- `q` or `Ctrl+C`: quit

## Text extraction and quality notes

PDF extraction is inherently imperfect because many PDFs encode text as positioned glyphs, not as plain logical paragraphs. To improve quality while preserving page boundaries, NvReader uses strict per-page fallback order:

1. `GetPlainText(nil)` for that page
2. `GetTextByRow()` for that page
3. `Content().Text` for that page

The app does not justify or aggressively reflow text anymore, to avoid breaking words. Depending on the source PDF generator, some pages can still contain minor artifacts, but page boundaries remain strict.

## Tests

```bash
go test ./...
```

## License

See `LICENSE`.