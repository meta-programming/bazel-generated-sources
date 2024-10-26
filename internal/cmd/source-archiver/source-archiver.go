// Program source-archiver packages a set of Go files into a .tar file.
package main

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/golang/glog"
	"golang.org/x/sync/errgroup"
)

var (
	spec       = flag.String("spec", "", "Path to Spec json.")
	outputPath = flag.String("output", "", "Output path of .tar to produce.")
)

func main() {
	flag.Parse()
	// Create and add some files to the archive.
	if err := run(); err != nil {
		glog.Exitf("error: %v", err)
	}
}

func run() error {
	if *outputPath == "" {
		return fmt.Errorf("must specify valid --output path")
	}
	if *spec == "" {
		return fmt.Errorf("must specify valid --spec path")
	}
	specBytes, err := os.ReadFile(*spec)
	if err != nil {
		return fmt.Errorf("error reading input spec: %w", err)
	}
	parsedSpec := &Spec{}
	if err := json.Unmarshal(specBytes, parsedSpec); err != nil {
		return fmt.Errorf("error parsing spec at %s: %w", *spec, err)
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	if err := writeTarEntries(parsedSpec, tw); err != nil {
		return fmt.Errorf("error writing tar entries: %w", err)
	}

	if err := tw.Close(); err != nil {
		return fmt.Errorf("error with Close: %w", err)
	}
	if err := os.WriteFile(*outputPath, buf.Bytes(), 0664); err != nil {
		return fmt.Errorf("I/O error writing output .tar file, but the actual contents were already produced successfully: %w", err)
	}
	return nil
}

func writeTarEntries(parsedSpec *Spec, tw *tar.Writer) error {
	var entries []tarEntry
	lock := sync.Mutex{}
	push := func(entry tarEntry) {
		lock.Lock()
		defer lock.Unlock()
		entries = append(entries, entry)
	}

	type fileToProcess struct {
		file         *File
		explicitName string
	}

	var files []*fileToProcess

	for _, pkg := range parsedSpec.PackageSpecs {
		for _, f := range pkg.Files {
			files = append(files, &fileToProcess{f, pkg.ImportPath + "/" + filepath.Base(f.Path)})
		}
	}

	eg := errgroup.Group{}
	for _, runfile := range files {
		runfile := runfile
		eg.Go(func() error {
			fileReader := func() ([]byte, error) {
				return os.ReadFile(runfile.file.Path)
			}
			contents, err := fileReader()
			if err != nil {
				return fmt.Errorf("error reading %q (short_path = %q): %w", runfile.file.Path, runfile.file.ShortPath, err)
			}
			fileInfo, err := os.Stat(runfile.file.Path)
			if err != nil {
				return fmt.Errorf("error calling os.Stat on %q (short_path = %q): %w", runfile.file.Path, runfile.file.ShortPath, err)
			}

			name := runfile.explicitName

			push(tarEntry{
				header: &tar.Header{
					Name: name,
					Mode: int64(fileInfo.Mode().Perm()),
					Size: int64(len(contents)),
				},
				contents: fileReader,
			})
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return fmt.Errorf("error generating tar metadata: %w", err)
	}
	// Make output deterministic by sorting filenames.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].header.Name < entries[j].header.Name
	})

	for _, entry := range entries {
		if err := tw.WriteHeader(entry.header); err != nil {
			return fmt.Errorf("error with WriteHeader: %w", err)
		}
		contents, err := entry.contents()
		if err != nil {
			return fmt.Errorf("error reading file while writing tar: %w", err)
		}
		if _, err := tw.Write(contents); err != nil {
			return fmt.Errorf("error with Write: %w", err)
		}
	}
	return nil
}

func nameInOutputArchive(runfile *File, workspaceName string, executable, repoMappingManifest *File, executableNameInArchive string) string {
	panic("blah")
}

type tarEntry struct {
	header   *tar.Header
	contents func() ([]byte, error)
}

// Spec contains a set of (import path, source file list) pairs.
type Spec struct {
	PackageSpecs []*GoPackageSourceFiles `json:"packages"`
}

// GoPackageSourceFiles describes all the source files in a Go package.
type GoPackageSourceFiles struct {
	ImportPath string  `json:"import_path"`
	Files      []*File `json:"files"`
}

// File contains information about a bazel File object.
//
// See https://bazel.build/rules/lib/builtins/File.
type File struct {
	IsDirectory bool        `json:"is_directory"`
	IsSource    bool        `json:"is_source"`
	Path        string      `json:"path"`
	ShortPath   string      `json:"short_path"`
	Owner       LabelString `json:"label"`
}

// LabelString is a superficial type for https://bazel.build/rules/lib/builtins/Label.html.
type LabelString string

// mapSlice applies a function to each element of a slice and returns a new slice.
func mapSlice[T, U any](slice []T, fn func(T) U) []U {
	result := []U{}
	for _, v := range slice {
		result = append(result, fn(v))
	}
	return result
}
