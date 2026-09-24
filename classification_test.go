package main

import (
	"sort"
	"testing"

	"github.com/smm-h/strictcli/go/strictcli"
	"github.com/stricttools/testisolation/go/hygiene"
)

// classification pins every command's strictcli effect classification and its
// `consequential` declaration. The table is the specification: changing a row
// here is the deliberate edit that a reclassification requires, and adding a
// command without adding its row fails the test.
//
// Reasoning, per the strictcli effects contract (§1: `read_only` means no
// user-visible or consequential mutation; §8.1: `consequential` means the act
// is worth interrupting someone for):
//
//   - scan -- read_only. It walks a directory tree, reads file bytes to
//     classify them and count lines, and writes nothing anywhere. Its only
//     output is stdout/stderr. The optional --config TOML is read, never
//     written.
//
// dirstat declares no consequential commands: it has exactly one command, it
// only reads, and a `read_only` command cannot be consequential at all.
var classification = map[string]struct {
	effect        string
	consequential bool
}{
	"scan": {strictcli.EffectReadOnly, false},
}

// collectCommands flattens the app's command tree into dotted paths.
func collectCommands(app *strictcli.App) map[string]*strictcli.Command {
	out := map[string]*strictcli.Command{}
	for name, cmd := range app.Commands() {
		out[name] = cmd
	}
	var walk func(prefix string, g *strictcli.Group)
	walk = func(prefix string, g *strictcli.Group) {
		for name, cmd := range g.Commands {
			out[prefix+name] = cmd
		}
		for name, sub := range g.Groups {
			walk(prefix+name+".", sub)
		}
	}
	for name, g := range app.Groups() {
		walk(name+".", g)
	}
	return out
}

func TestCommandClassificationIsPinned(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))

	cmds := collectCommands(newApp())

	var names []string
	for name := range cmds {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		want, ok := classification[name]
		if !ok {
			t.Errorf("command %q is registered but has no pinned classification; add a row to classification with the reasoning", name)
			continue
		}
		if cmds[name].Effect != want.effect {
			t.Errorf("command %q: effect = %q, pinned %q", name, cmds[name].Effect, want.effect)
		}
		if cmds[name].Consequential != want.consequential {
			t.Errorf("command %q: consequential = %v, pinned %v", name, cmds[name].Consequential, want.consequential)
		}
	}
	for name := range classification {
		if _, ok := cmds[name]; !ok {
			t.Errorf("classification pins %q but no such command is registered", name)
		}
	}
}

// TestNoReservedGlobalFlagNames guards the framework's reserved quartet at the
// app level. Command-level flags are covered implicitly: strictcli panics at
// registration for a reserved name anywhere, and newApp() registers everything.
func TestNoReservedGlobalFlagNames(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))

	reserved := map[string]bool{"dry-run": true, "yes": true, "quiet": true, "verbose": true}
	for _, f := range newApp().GlobalFlags() {
		if reserved[f.Name] {
			t.Errorf("global flag %q is reserved by the framework", f.Name)
		}
	}
}
