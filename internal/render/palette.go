package render

import (
	"os"
	"strconv"
	"strings"

	"github.com/stricttools/dirstat/internal/config"
)

// palette holds ready-to-print ANSI sequences. When disabled, every field is
// the empty string, so disabled output is byte-identical to non-TTY output.
type palette struct {
	enabled bool

	reset, bold, dim string

	textFG, textBG     string
	binaryFG, binaryBG string
	headerFG, headerBG string
	statLabel          string
	statValue          string
	errorC             string
	border             string
}

func fgSeq(code string) string {
	if code == "" {
		return ""
	}
	return "\033[38;5;" + code + "m"
}

func bgSeq(code string) string {
	if code == "" {
		return ""
	}
	return "\033[48;5;" + code + "m"
}

func newPalette(enabled bool, t config.Theme) palette {
	if !enabled {
		return palette{}
	}
	return palette{
		enabled:   true,
		reset:     "\033[0m",
		bold:      "\033[1m",
		dim:       "\033[2m",
		textFG:    fgSeq(t.TextFG),
		textBG:    bgSeq(t.TextBG),
		binaryFG:  fgSeq(t.BinaryFG),
		binaryBG:  bgSeq(t.BinaryBG),
		headerFG:  fgSeq(t.HeaderFG),
		headerBG:  bgSeq(t.HeaderBG),
		statLabel: fgSeq(t.StatLabel),
		statValue: fgSeq(t.StatValue),
		errorC:    fgSeq(t.Error),
		border:    fgSeq(t.Border),
	}
}

// DetectThemeName picks "dark" or "light" from the COLORFGBG environment
// variable ("fg;bg"). Backgrounds 7, 15, and 9-14 indicate a light terminal;
// anything else (or no clear signal) defaults to dark.
func DetectThemeName() string {
	colorfgbg := os.Getenv("COLORFGBG")
	if colorfgbg == "" {
		return "dark"
	}
	parts := strings.Split(colorfgbg, ";")
	if len(parts) < 2 {
		return "dark"
	}
	bg, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return "dark"
	}
	if bg == 7 || bg == 15 || (bg >= 9 && bg <= 14) {
		return "light"
	}
	return "dark"
}
