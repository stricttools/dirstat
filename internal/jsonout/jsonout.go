// Package jsonout builds the machine-readable scan document. The document is
// the scan command's declared payload: the framework carries it as the
// envelope's payload member under --json and validates it against Schema
// before writing it. The shape is a consumer contract (R34): field names are
// stable, values are raw integers, no ANSI codes, and evolution is
// additive-only.
package jsonout

import (
	"bytes"
	"encoding/json"
	"strconv"

	"github.com/stricttools/dirstat/internal/scan"
	"github.com/smm-h/strictcli/go/strictcli"
)

// Selection reports which stats were selected via --stats. Unselected stats
// are omitted from group objects.
type Selection map[string]bool

type summaryJSON struct {
	Directories           int `json:"directories"`
	Files                 int `json:"files"`
	FilesWithoutExtension int `json:"files_without_extension"`
	MaxDepth              int `json:"max_depth"`
	Symlinks              int `json:"symlinks"`
	Executables           int `json:"executables"`
	UniqueFormats         int `json:"unique_formats"`
	FilesSniffed          int `json:"files_sniffed"`
	Unreadable            int `json:"unreadable"`
}

type groupJSON struct {
	g   scan.Group
	sel Selection
}

// MarshalJSON writes the group fields in the documented, stable order.
// Unselected stats are omitted; LOC fields are null for binary groups.
func (gj groupJSON) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')

	writeKey := func(key string) {
		if b.Len() > 1 {
			b.WriteByte(',')
		}
		b.WriteByte('"')
		b.WriteString(key)
		b.WriteString(`":`)
	}
	writeInt := func(key string, v int64) {
		writeKey(key)
		b.WriteString(strconv.FormatInt(v, 10))
	}
	writeLOC := func(key string, v int64) {
		writeKey(key)
		if gj.g.HasLOC {
			b.WriteString(strconv.FormatInt(v, 10))
		} else {
			b.WriteString("null")
		}
	}

	name, err := json.Marshal(gj.g.Format)
	if err != nil {
		return nil, err
	}
	writeKey("format")
	b.Write(name)
	writeKey("text")
	b.WriteString(strconv.FormatBool(gj.g.Text))

	if gj.sel["count"] {
		writeInt("count", int64(gj.g.Count))
	}
	if gj.sel["total-size"] {
		writeInt("total_size", gj.g.TotalSize)
	}
	if gj.sel["min-size"] {
		writeInt("min_size", gj.g.MinSize)
	}
	if gj.sel["max-size"] {
		writeInt("max_size", gj.g.MaxSize)
	}
	if gj.sel["avg-size"] {
		writeInt("avg_size", gj.g.AvgSize)
	}
	if gj.sel["total-loc"] {
		writeLOC("total_loc", gj.g.TotalLOC)
	}
	if gj.sel["min-loc"] {
		writeLOC("min_loc", gj.g.MinLOC)
	}
	if gj.sel["max-loc"] {
		writeLOC("max_loc", gj.g.MaxLOC)
	}
	if gj.sel["avg-loc"] {
		writeLOC("avg_loc", gj.g.AvgLOC)
	}

	b.WriteByte('}')
	return b.Bytes(), nil
}

// Document is the scan command's machine payload. The binary's own version is
// deliberately absent: the envelope carries it as app_version, and one fact
// belongs in one place.
type Document struct {
	Root    string      `json:"root"`
	Method  string      `json:"method"`
	Config  string      `json:"config,omitempty"`
	Summary summaryJSON `json:"summary"`
	Groups  []groupJSON `json:"groups"`
	NoExt   *[]string   `json:"no_extension_files,omitempty"`
}

// Build assembles the document for a scan result. groups must already be
// sorted per --sort-by/--sort-order. listNoExt controls the presence of the
// no_extension_files field. configPath is the absolute path of the scan
// config file in effect; when empty the additive "config" field is omitted
// entirely (R46).
func Build(configPath string, res *scan.Result, groups []scan.Group, sel Selection, listNoExt bool) Document {
	s := res.Summary
	d := Document{
		Root:   res.Root,
		Method: res.Method,
		Config: configPath,
		Summary: summaryJSON{
			Directories:           s.Directories,
			Files:                 s.Files,
			FilesWithoutExtension: s.FilesWithoutExtension,
			MaxDepth:              s.MaxDepth,
			Symlinks:              s.Symlinks,
			Executables:           s.Executables,
			UniqueFormats:         s.UniqueFormats,
			FilesSniffed:          s.FilesSniffed,
			Unreadable:            s.Unreadable,
		},
		Groups: make([]groupJSON, len(groups)),
	}
	for i, g := range groups {
		d.Groups[i] = groupJSON{g: g, sel: sel}
	}
	if listNoExt {
		files := res.NoExtFiles
		if files == nil {
			files = []string{}
		}
		d.NoExt = &files
	}

	return d
}

// intStat is a selected integer stat: present only when --stats asked for it.
func intStat() map[string]interface{} { return strictcli.SchemaType("integer") }

// locStat is a selected LOC stat: null for binary groups, which is why the
// declaration is a type list rather than a plain integer.
func locStat() map[string]interface{} { return strictcli.SchemaType("integer", "null") }

// Schema is the scan command's declared payload schema (strictcli §19.5). It
// states the same contract R34/R35 states in prose, in the form the framework
// enforces at emission: a document deviating from it fails the run instead of
// shipping a wrong shape to a consumer.
//
// Every stat property is optional because --stats selects which ones the group
// objects carry; only the two identity fields are required of a group. The
// additive fields "config" (R46) and "no_extension_files" are optional for the
// same reason they are omitempty on the struct.
var Schema = strictcli.SchemaObject(
	map[string]interface{}{
		"root":   strictcli.SchemaType("string"),
		"method": strictcli.SchemaEnum("ext", "type", "hybrid"),
		"config": strictcli.SchemaType("string"),
		"summary": strictcli.SchemaObject(
			map[string]interface{}{
				"directories":             intStat(),
				"files":                   intStat(),
				"files_without_extension": intStat(),
				"max_depth":               intStat(),
				"symlinks":                intStat(),
				"executables":             intStat(),
				"unique_formats":          intStat(),
				"files_sniffed":           intStat(),
				"unreadable":              intStat(),
			},
			[]string{
				"directories", "files", "files_without_extension", "max_depth",
				"symlinks", "executables", "unique_formats", "files_sniffed",
				"unreadable",
			},
			false,
		),
		"groups": strictcli.SchemaArray(strictcli.SchemaObject(
			map[string]interface{}{
				"format":     strictcli.SchemaType("string"),
				"text":       strictcli.SchemaType("boolean"),
				"count":      intStat(),
				"total_size": intStat(),
				"min_size":   intStat(),
				"max_size":   intStat(),
				"avg_size":   intStat(),
				"total_loc":  locStat(),
				"min_loc":    locStat(),
				"max_loc":    locStat(),
				"avg_loc":    locStat(),
			},
			[]string{"format", "text"},
			false,
		)),
		"no_extension_files": strictcli.SchemaArray(strictcli.SchemaType("string")),
	},
	[]string{"root", "method", "summary", "groups"},
	false,
)
