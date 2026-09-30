package render

import (
	"fmt"

	"github.com/stricttools/dirstat/internal/config"
)

// FormatError renders one stderr error line: "error: <message>\n". When
// colored is true (stderr is a TTY and --colors is true), the "error:"
// prefix carries the theme's error color (R30); otherwise the result is
// plain text, byte-identical to non-TTY output.
func FormatError(colored bool, theme config.Theme, format string, args ...interface{}) string {
	msg := fmt.Sprintf(format, args...)
	if !colored {
		return "error: " + msg + "\n"
	}
	p := newPalette(true, theme)
	return p.errorC + "error:" + p.reset + " " + msg + "\n"
}
