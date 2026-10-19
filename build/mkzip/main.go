// Command mkzip writes a reproducible zip of a folder for build/release.ps1.
//
// Entries are sorted by byte order, use forward slashes, carry a fixed
// timestamp and are deflated by the Go toolchain pinned in go.mod, so the
// same input files give the same zip bytes on any machine (PowerShell's
// own zip support compresses differently on Windows PowerShell 5.1 and
// PowerShell 7).
//
// Usage: go run ./build/mkzip -dir <folder> -out <file.zip>
package main

import (
	"archive/zip"
	"compress/flate"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

func main() {
	dir := flag.String("dir", "", "folder to zip (its contents go to the zip root)")
	out := flag.String("out", "", "zip file to create")
	flag.Parse()
	if *dir == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: mkzip -dir <folder> -out <file.zip>")
		os.Exit(2)
	}
	if err := run(*dir, *out); err != nil {
		fmt.Fprintln(os.Stderr, "mkzip:", err)
		os.Exit(1)
	}
}

func run(dir, out string) error {
	var names []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			rel, err := filepath.Rel(dir, p)
			if err != nil {
				return err
			}
			names = append(names, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(names)

	f, err := os.OpenFile(out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	zw.RegisterCompressor(zip.Deflate, func(w io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(w, flate.BestCompression)
	})
	fixed := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, name := range names {
		fh := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: fixed}
		fh.SetMode(0o644)
		w, err := zw.CreateHeader(fh)
		if err != nil {
			return err
		}
		src, err := os.Open(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil {
			return err
		}
		_, err = io.Copy(w, src)
		src.Close()
		if err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
