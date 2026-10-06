// Command dirstat: Fast, single-binary directory statistics CLI: every file
// under a tree grouped by format, with counts, sizes, and lines of code, as a
// colored terminal table or as JSON.
//
// The binary registers a single command, scan, via strictcli and dispatches to
// its handler. Machine output is the framework's --json envelope, whose payload
// is the scan document built by internal/jsonout.
package main

import (
	"github.com/stricttools/strictcli/go/strictcli"
)

// newApp builds the fully-registered dirstat app. It is separate from main so
// tests can construct the same app and assert over its registration (see
// classification_test.go).
func newApp() *strictcli.App {
	app := strictcli.NewApp("dirstat", Version,
		"Fast, single-binary directory statistics CLI: every file under a tree grouped by format, with counts, sizes, and lines of code, as a colored terminal table or as JSON")
	registerScanCmd(app)
	return app
}

func main() {
	newApp().Run()
}
