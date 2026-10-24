package wiki

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
)

// bundleName is the archive a rendered site offers for download. It sits at the
// render root, so a bundle that already owns that path fails the render rather
// than losing the file to the export.
const bundleName = "okf-bundle.zip"

// errBundleCollision marks a content root that already holds a file at the
// export's path. A tree fails rather than publishing without that wiki, because
// a collision is a source defect rather than a stale checkout to skip.
var errBundleCollision = errors.New("bundle export")

// bundleCollision refuses a content root that already holds the archive's name,
// rather than overwriting a reader's file with the export.
func bundleCollision(dir string) error {
	if _, err := os.Stat(filepath.Join(dir, bundleName)); err == nil {
		return fmt.Errorf("%w: %s holds %s, which is reserved for the export; rename that file", errBundleCollision, dir, bundleName)
	}
	return nil
}

// writeBundle writes the content root beside the rendered site as a zip, so the
// whole wiki downloads as one file. Entries keep their paths under a folder
// named after the content directory, so extracting yields one directory rather
// than spraying files into the reader's working directory. The name comes from
// the path as given, so a linked wiki keeps the name it is linked under rather
// than its checkout's.
//
// What goes in is the bundle the render read, not the rendered output: Markdown
// keeps its frontmatter and an asset keeps its bytes, so the archive is an OKF
// bundle in its own right rather than a copy of the site.
func (r *Renderer) writeBundle(dest string) (err error) {
	real, files, err := bundleFiles(r.cfg.Content)
	if err != nil {
		return err
	}
	folder := bundleFolder(r.cfg.Content)
	target := filepath.Join(dest, bundleName)
	f, err := os.Create(target)
	if err != nil {
		return err
	}
	// A failed export leaves no half-written archive behind for a server to
	// hand out as though it were the bundle.
	defer func() {
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(target)
		}
	}()
	zw := zip.NewWriter(f)
	defer func() {
		if closeErr := zw.Close(); err == nil {
			err = closeErr
		}
	}()
	for _, rel := range files {
		src := filepath.Join(real, filepath.FromSlash(rel))
		info, err := os.Stat(src)
		if err != nil {
			return err
		}
		if err := addBundleEntry(zw, path.Join(folder, rel), src, info); err != nil {
			return err
		}
	}
	return nil
}

// bundleFolder names the archive's top-level directory after the content path
// as given, so a linked wiki keeps the name it is linked under. A path that
// names no directory of its own, or whose base could climb out of the
// extraction directory, falls back to a fixed name.
func bundleFolder(content string) string {
	folder := filepath.Base(filepath.Clean(content))
	if folder == "" || folder == "." || folder == ".." || folder == string(filepath.Separator) {
		return "okf-bundle"
	}
	return folder
}

// addBundleEntry keeps the file's mode, so an executable script stays
// executable after extraction.
func addBundleEntry(zw *zip.Writer, name, src string, info fs.FileInfo) error {
	hdr := &zip.FileHeader{
		Name:     name,
		Method:   zip.Deflate,
		Modified: info.ModTime(),
	}
	hdr.SetMode(info.Mode().Perm())
	w, err := zw.CreateHeader(hdr)
	if err != nil {
		return err
	}
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}
