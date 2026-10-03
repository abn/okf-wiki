package wiki

import (
	"os"
	"path/filepath"
)

// swapDir atomically replaces dest with the contents of a freshly built
// directory. build is handed an empty temporary directory to fill; whatever it
// writes there becomes dest, and the temporary directory is removed either way.
//
// The point is that a request never sees a half-written tree. A render that
// starts by deleting dest and then writes into it leaves a window with no file
// at all, so a reader at that instant gets a 404; it also exposes a truncated
// file.
//
// dest is moved aside rather than deleted, so dest is never absent: the two
// renames below leave dest pointing at the old tree until the instant the new
// one takes its place. os.Rename cannot overwrite a non-empty directory, which
// is why the old tree is moved out of the way first instead of being replaced
// in one call. A reader that opened the old tree keeps a working handle to it
// until the aside copy is removed, so a request in flight during the swap reads
// a whole tree, old or new.
//
// The temporary directory is a sibling of dest so both renames stay on one
// filesystem, and its removal is deterministic.
func swapDir(dest string, build func(tmp string) error) error {
	parent := filepath.Dir(dest)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(parent, "."+filepath.Base(dest)+".tmp-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	if err := build(tmp); err != nil {
		return err
	}
	// Move the old tree aside first, so dest is never missing. A failure here
	// leaves dest as it was, so a failed render keeps serving the last good tree
	// rather than an empty directory.
	old := dest + ".old-" + filepath.Base(tmp)
	if _, err := os.Lstat(dest); err == nil {
		if err := os.Rename(dest, old); err != nil {
			return err
		}
		defer os.RemoveAll(old)
	}
	if err := os.Rename(tmp, dest); err != nil {
		// Put the old tree back, so a failed swap does not leave dest missing.
		_ = os.Rename(old, dest)
		return err
	}
	return nil
}

// sitePath is SiteDir for a base already known to be valid, which the tree's own
// bases are: they are built from the configured base, so they cannot carry a
// path segment SiteDir would reject.
func (c Config) sitePath(out string) string {
	dir, err := c.SiteDir(out)
	if err != nil {
		// Unreachable for a base this package built. Falling back to the output
		// root keeps the caller free of an error it cannot cause.
		return out
	}
	return dir
}
