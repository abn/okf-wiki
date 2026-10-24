package wiki

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// bundleFiles lists the files a content root contributes to the site, the page
// walk and the archive, as paths relative to the root and in sorted order. One
// list keeps a file that serves the same file that downloads.
//
// Dotfiles and dot-directories are skipped: a bundle directory is a real working
// tree as often as not, and nothing in a wiki should publish a .git directory or
// a .env.
//
// The root is resolved first, because WalkDir treats a symlinked starting point
// as a file; a bundle reached through a link would otherwise yield nothing.
func bundleFiles(root string) (string, []string, error) {
	real, err := resolveRoot(root)
	if err != nil {
		return "", nil, err
	}
	var files []string
	err = filepath.WalkDir(real, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(real, p)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return "", nil, err
	}
	sort.Strings(files)
	return real, files, nil
}
