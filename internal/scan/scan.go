// Package scan walks a directory tree, classifies files via a worker pool,
// and aggregates per-format statistics.
package scan

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"os"
	"runtime"
	"sort"
	"sync"

	"github.com/stricttools/dirstat/internal/classify"
)

// Values for Options.Ignored.
const (
	IgnoredInclude = "include"
	IgnoredExclude = "exclude"
	IgnoredOnly    = "only"
)

// Values for Options.Hidden.
const (
	HiddenInclude = "include"
	HiddenExclude = "exclude"
)

// Values for Options.TypeFilter.
const (
	TypeText   = "text"
	TypeBinary = "binary"
	TypeBoth   = "both"
)

// Values for Options.Formats.
const (
	FormatsRaw       = classify.FormatsRaw
	FormatsCanonical = classify.FormatsCanonical
)

// Options configures a scan.
type Options struct {
	Root       string   // absolute path of the directory to scan
	Method     string   // classify.MethodExt, MethodType, or MethodHybrid
	Formats    string   // FormatsRaw or FormatsCanonical: how group names are spelled
	Depth      int      // max directory depth below root; -1 = unlimited; root is depth 0
	Exclude    []string // exact base names to skip (dirs and files)
	Ignored    string   // IgnoredInclude, IgnoredExclude, or IgnoredOnly
	Hidden     string   // HiddenInclude or HiddenExclude
	TypeFilter string   // TypeText, TypeBinary, or TypeBoth
	NeedLOC    bool     // count lines of code for text files
	ListNoExt  bool     // collect relative paths of extensionless files
	TextExts   map[string]struct{}
	TextMimes  map[string]struct{}
	Aliases    map[string]string // canonical format aliases; read in FormatsCanonical only
	Workers    int               // worker pool size; 0 = runtime.GOMAXPROCS(0)
}

// Summary holds the tree-wide summary statistics.
type Summary struct {
	Directories           int // counted directories, including the root
	Files                 int // readable files that passed traversal filters
	FilesWithoutExtension int // extensionless files that passed all filters including --type
	MaxDepth              int // deepest counted directory (root is 0)
	Symlinks              int // symlinks encountered in traversed directories (never followed)
	Executables           int // regular files with any execute permission bit
	UniqueFormats         int // number of groups after the --type filter
	FilesSniffed          int // content-sniff operations performed
	Unreadable            int // directories and files skipped due to read/stat errors
}

// Group is the aggregate statistics for one format.
type Group struct {
	Format string
	Text   bool
	Pseudo bool // true for the "(singletons)" aggregate row (render-only)

	Count     int
	TotalSize int64
	MinSize   int64
	MaxSize   int64
	AvgSize   int64 // rounded to nearest integer

	HasLOC   bool // LOC values are valid (text group with LOC computed)
	TotalLOC int64
	MinLOC   int64
	MaxLOC   int64
	AvgLOC   int64 // rounded to nearest integer
}

// Result is the outcome of a scan. Groups are ordered by format name
// ascending; callers apply their own sort.
type Result struct {
	Root       string
	Method     string
	Summary    Summary
	Groups     []Group
	NoExtFiles []string // relative paths, sorted; filled only when Options.ListNoExt
}

// fileEntry is a file discovered by the walker, pending classification.
type fileEntry struct {
	rel  string // slash-separated path relative to root
	name string // base name
	size int64
	exec bool
}

// fileResult is the classification outcome for one fileEntry.
type fileResult struct {
	group      string
	text       bool
	noExt      bool
	sniffed    bool
	unreadable bool
	loc        int64
}

// Scan walks opts.Root and returns aggregated statistics. It returns an
// error only if the root itself cannot be read; unreadable entries below the
// root are counted in Summary.Unreadable instead.
func Scan(opts Options) (*Result, error) {
	w := &walker{opts: &opts, excl: make(map[string]struct{})}
	for _, e := range opts.Exclude {
		w.excl[e] = struct{}{}
	}
	if opts.Ignored != IgnoredInclude {
		w.ign = newIgnoreMatcher(opts.Root)
	}

	w.sum.Directories = 1 // the root itself
	if err := w.walkRoot(); err != nil {
		return nil, fmt.Errorf("reading %s: %w", opts.Root, err)
	}

	results := classifyFiles(&opts, w.files)

	res := &Result{Root: opts.Root, Method: opts.Method, Summary: w.sum}
	aggregate(&opts, w.files, results, res)
	return res, nil
}

// classifyFiles runs classification (and LOC counting) over the discovered
// files in a worker pool. Results are indexed by file position, so the
// outcome is deterministic regardless of scheduling.
func classifyFiles(opts *Options, files []fileEntry) []fileResult {
	results := make([]fileResult, len(files))
	if len(files) == 0 {
		return results
	}

	workers := opts.Workers
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	if workers > len(files) {
		workers = len(files)
	}

	cls := classify.New(opts.Method, opts.Formats, opts.TextExts, opts.TextMimes, opts.Aliases)
	indices := make(chan int)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, 64*1024)
			for i := range indices {
				results[i] = processFile(opts, cls, files[i], buf)
			}
		}()
	}
	for i := range files {
		indices <- i
	}
	close(indices)
	wg.Wait()
	return results
}

func processFile(opts *Options, cls *classify.Classifier, fe fileEntry, buf []byte) fileResult {
	abs := absJoin(opts.Root, fe.rel)
	c, err := cls.File(abs, fe.name, fe.size)
	r := fileResult{group: c.Group, text: c.Text, noExt: c.NoExt, sniffed: c.Sniffed}
	if err != nil {
		r.unreadable = true
		return r
	}
	if opts.NeedLOC && c.Text {
		loc, err := countLOC(abs, buf)
		if err != nil {
			r.unreadable = true
			return r
		}
		r.loc = loc
	}
	return r
}

// agg accumulates statistics for one group.
type agg struct {
	text    bool
	count   int
	totSize int64
	minSize int64
	maxSize int64
	hasLOC  bool
	totLOC  int64
	minLOC  int64
	maxLOC  int64
}

func aggregate(opts *Options, files []fileEntry, results []fileResult, res *Result) {
	aggs := make(map[string]*agg)
	for i, r := range results {
		if r.sniffed {
			res.Summary.FilesSniffed++
		}
		if r.unreadable {
			res.Summary.Unreadable++
			continue
		}
		res.Summary.Files++
		if files[i].exec {
			res.Summary.Executables++
		}
		if opts.TypeFilter == TypeText && !r.text {
			continue
		}
		if opts.TypeFilter == TypeBinary && r.text {
			continue
		}
		if r.noExt {
			res.Summary.FilesWithoutExtension++
			if opts.ListNoExt {
				res.NoExtFiles = append(res.NoExtFiles, files[i].rel)
			}
		}
		a, ok := aggs[r.group]
		if !ok {
			a = &agg{text: r.text, hasLOC: r.text && opts.NeedLOC}
			aggs[r.group] = a
		}
		size := files[i].size
		if a.count == 0 {
			a.minSize, a.maxSize = size, size
			a.minLOC, a.maxLOC = r.loc, r.loc
		} else {
			a.minSize = min(a.minSize, size)
			a.maxSize = max(a.maxSize, size)
			a.minLOC = min(a.minLOC, r.loc)
			a.maxLOC = max(a.maxLOC, r.loc)
		}
		a.count++
		a.totSize += size
		a.totLOC += r.loc
	}

	res.Summary.UniqueFormats = len(aggs)
	sort.Strings(res.NoExtFiles)

	names := make([]string, 0, len(aggs))
	for name := range aggs {
		names = append(names, name)
	}
	sort.Strings(names)
	res.Groups = make([]Group, 0, len(names))
	for _, name := range names {
		a := aggs[name]
		g := Group{
			Format:    name,
			Text:      a.text,
			Count:     a.count,
			TotalSize: a.totSize,
			MinSize:   a.minSize,
			MaxSize:   a.maxSize,
			AvgSize:   roundDiv(a.totSize, int64(a.count)),
			HasLOC:    a.hasLOC,
		}
		if a.hasLOC {
			g.TotalLOC = a.totLOC
			g.MinLOC = a.minLOC
			g.MaxLOC = a.maxLOC
			g.AvgLOC = roundDiv(a.totLOC, int64(a.count))
		}
		res.Groups = append(res.Groups, g)
	}
}

// roundDiv divides total by count, rounding to the nearest integer (not
// floored). count must be positive.
func roundDiv(total, count int64) int64 {
	return int64(math.Round(float64(total) / float64(count)))
}

// countLOC counts lines of code: the number of '\n' bytes, plus 1 if the
// file is non-empty and does not end with '\n'. buf is a reusable read
// buffer.
func countLOC(path string, buf []byte) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	var loc int64
	var last byte
	empty := true
	for {
		n, err := f.Read(buf)
		if n > 0 {
			empty = false
			chunk := buf[:n]
			loc += int64(bytes.Count(chunk, []byte{'\n'}))
			last = chunk[n-1]
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}
	}
	if !empty && last != '\n' {
		loc++
	}
	return loc, nil
}
