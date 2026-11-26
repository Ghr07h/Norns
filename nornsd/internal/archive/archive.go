// Package archive creates bounded-memory, self-contained directory downloads.
package archive

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type File struct {
	*os.File
	Filename  string
	directory string
}

func (f *File) Close() error {
	return errors.Join(f.File.Close(), os.RemoveAll(f.directory))
}

// Create snapshots Norns/nornsd databases without interpreting their schemas.
// The caller must prevent runtime and View writes until this function returns.
func Create(ctx context.Context, source, rootPath, prefix string) (_ *File, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		return nil, err
	}
	rootPath, err = filepath.EvalSymlinks(rootPath)
	if err != nil {
		return nil, err
	}
	relativeSource, err := filepath.Rel(rootPath, source)
	if err != nil || relativeSource == ".." || strings.HasPrefix(relativeSource, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("archive source is outside the nornsd root")
	}
	temporary, err := temporaryOutside(rootPath)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(temporary)
		}
	}()
	filename := prefix + "-" + time.Now().UTC().Format("20060102T150405.000000000Z") + ".tar.gz"
	file, err := os.OpenFile(filepath.Join(temporary, filename), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = file.Close()
		}
	}()
	gz := gzip.NewWriter(contextWriter{ctx: ctx, writer: file})
	tw := tar.NewWriter(gz)
	top := filepath.Base(source)
	if top == "." || top == string(filepath.Separator) {
		top = "nornsd-root"
	}
	err = filepath.WalkDir(source, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(source, filename)
		if err != nil {
			return err
		}
		rootRelative, err := filepath.Rel(rootPath, filename)
		if err != nil {
			return err
		}
		if exclude(rootRelative) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		link := ""
		if info.Mode()&os.ModeSymlink != 0 {
			link, err = os.Readlink(filename)
			if err != nil {
				return err
			}
		} else if !info.IsDir() && !info.Mode().IsRegular() {
			// Sockets, devices and FIFOs are live process resources, not files.
			return nil
		}
		header, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(filepath.Join(top, rel))
		if info.IsDir() {
			header.Name += "/"
		}
		if !info.Mode().IsRegular() {
			return tw.WriteHeader(header)
		}
		readPath := filename
		if database(rootRelative) {
			readPath = filepath.Join(temporary, "snapshot.sqlite")
			defer os.Remove(readPath)
			if err := snapshotSQLite(ctx, filename, readPath); err != nil {
				return fmt.Errorf("snapshot %s: %w", rel, err)
			}
			snapshotInfo, err := os.Stat(readPath)
			if err != nil {
				return err
			}
			header.Size = snapshotInfo.Size()
		}
		input, err := os.Open(readPath)
		if err != nil {
			return err
		}
		defer input.Close()
		opened, err := input.Stat()
		if err != nil {
			return err
		}
		if !database(rootRelative) && (!opened.Mode().IsRegular() || !os.SameFile(info, opened)) {
			return fmt.Errorf("file changed while archiving: %s", rel)
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		_, err = io.CopyN(tw, contextReader{ctx: ctx, reader: input}, header.Size)
		return err
	})
	err = errors.Join(err, tw.Close(), gz.Close())
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	return &File{File: file, Filename: filename, directory: temporary}, nil
}

func temporaryOutside(rootPath string) (string, error) {
	dir, err := os.MkdirTemp("", "nornsd-archive-")
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	rel, err := filepath.Rel(rootPath, resolved)
	if err == nil && (rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
		return dir, nil
	}
	_ = os.RemoveAll(dir)
	parent := filepath.Dir(rootPath)
	if parent == rootPath {
		return "", fmt.Errorf("cannot create a temporary archive outside the root")
	}
	return os.MkdirTemp(parent, ".nornsd-archive-")
}

func database(rel string) bool {
	p := filepath.ToSlash(rel)
	return p == ".nornsd/service/nornsd.sqlite" || p == ".norns/norns.sqlite" || strings.HasSuffix(p, "/.norns/norns.sqlite")
}

func exclude(rel string) bool {
	p := filepath.ToSlash(rel)
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if strings.HasSuffix(p, suffix) && database(strings.TrimSuffix(p, suffix)) {
			return true
		}
	}
	parts := strings.Split(p, "/")
	for i, part := range parts {
		if part == ".norns.lock" || part == ".norns.lifecycle.lock" {
			return true
		}
		if part == ".nornsd" && i+1 < len(parts) {
			next := parts[i+1]
			if next == "staging" || next == "configs.stage" || next == "configs.old" {
				return true
			}
			if next == "queue_record" && i+2 < len(parts) && strings.HasPrefix(parts[i+2], ".export-") && strings.HasSuffix(parts[i+2], ".tmp") {
				return true
			}
			if next == "service" && i+2 < len(parts) && (parts[i+2] == "nornsd_lock" || parts[i+2] == "owner.json") {
				return true
			}
			if next == "subjects" && i+3 < len(parts) && parts[i+3] == "staging" {
				return true
			}
		}
	}
	return false
}

func snapshotSQLite(ctx context.Context, source, destination string) error {
	uri := (&url.URL{Scheme: "file", Path: filepath.ToSlash(source)}).String()
	db, err := sql.Open("sqlite", uri+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.ExecContext(ctx, `VACUUM main INTO ?`, destination)
	return err
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

type contextWriter struct {
	ctx    context.Context
	writer io.Writer
}

func (w contextWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	return w.writer.Write(p)
}
