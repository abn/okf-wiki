package wiki

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// WatchOptions configures a reload loop over a bundle.
type WatchOptions struct {
	// Content is the bundle directory to watch.
	Content string
	// ThemeDir is a theme directory layered over the embedded default, watched
	// as well when one is in use.
	ThemeDir string
	// Interval is the poll interval. It defaults to a second.
	Interval time.Duration
	// Render is called once per detected change. It runs on the watch
	// goroutine, so a slow render delays the next poll rather than overlapping
	// with it.
	Render func() error
	// Logf reports progress and failure. It defaults to stderr.
	Logf func(format string, args ...any)
}

// Watch renders again whenever the bundle, or a theme layered over it, changes,
// and returns when ctx is done.
//
// It polls rather than subscribing to filesystem notifications. A bundle is a
// few hundred small files, so a walk is cheap; an editor may save through a
// temporary file and a rename, which a notification reports as several events
// that each need coalescing; and a poll needs no dependency and no bookkeeping
// for the directories a new section introduces. The interval is also the
// debounce: a burst of writes is one change by the next tick.
func Watch(ctx context.Context, opts WatchOptions) {
	interval := opts.Interval
	if interval <= 0 {
		interval = time.Second
	}
	logf := opts.Logf
	if logf == nil {
		logf = func(format string, args ...any) { fmt.Fprintf(os.Stderr, format, args...) }
	}
	roots := []string{opts.Content}
	if opts.ThemeDir != "" {
		roots = append(roots, opts.ThemeDir)
	}

	last := watchSignature(roots)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		next := watchSignature(roots)
		if next == last {
			continue
		}
		last = next
		logf("okf-wiki: change detected, re-rendering\n")
		if err := opts.Render(); err != nil {
			// A bundle saved mid-edit fails to parse. The server keeps serving
			// the last good render, and the next save is tried again, so a
			// transient error is not fatal and does not need a restart.
			logf("okf-wiki: re-render failed, still serving the last good one: %v\n", err)
		}
	}
}

// watchSignature hashes the path, size and modification time of every file under
// each root. Dotfiles and dot-directories are left out, matching what a render
// publishes, so a change inside .git is not a change to the site and does not
// trigger a render. Directories are left out too: adding a file changes the
// parent's modification time, so hashing the directory would make a skipped
// dotfile a change. A new or removed file changes its own entry either way, and
// a root that cannot be read contributes nothing, so a theme directory that
// appears later is a change rather than a permanent difference.
func watchSignature(roots []string) string {
	h := sha256.New()
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || p == root {
				return nil
			}
			if strings.HasPrefix(d.Name(), ".") {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				return nil
			}
			info, ierr := d.Info()
			if ierr != nil {
				return nil
			}
			fmt.Fprintf(h, "%s\x00%d\x00%d\x00", p, info.Size(), info.ModTime().UnixNano())
			return nil
		})
		fmt.Fprintf(h, "%s\x00", root)
	}
	return hex.EncodeToString(h.Sum(nil))
}
