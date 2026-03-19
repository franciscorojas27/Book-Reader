# goPdf

goPdf is a simple Go application for reading and navigating through PDF files in the terminal. It uses the `dslipak/pdf` library for PDF parsing and `eiannone/keyboard` for keyboard input handling.

## Features
- Navigate through PDF pages using keyboard keys.
- Display server-like status messages for a dynamic console experience.
- Clear and formatted text output for better readability.

## Usage
1. Install Go on your system.
2. Clone this repository.
3. Run the application with a PDF file as an argument:
   ```bash
   go run . <path-to-pdf>
   ```

### Keyboard Controls
- `j`: Go to the next page.
- `k`: Go to the previous page.
- `g`: Go to a specific page.
- `q`: Quit the application.

## Dependencies
- `dslipak/pdf`
- `eiannone/keyboard`
- `fatih/color`

## License
This project is licensed under the MIT License. See the LICENSE file for details.