package main

import (
	"io"
	"os"
	pathpkg "path"

	"nxaterm/internal/xfer"

	"github.com/pkg/sftp"
)

// sftpFS adapta um cliente SFTP para o motor de transferência.
type sftpFS struct{ c *sftp.Client }

func sftpEntry(fi os.FileInfo) xfer.Entry {
	return xfer.Entry{Name: fi.Name(), Size: fi.Size(), IsDir: fi.IsDir(), Mode: fi.Mode(), ModTime: fi.ModTime()}
}

func (f sftpFS) List(dir string) ([]xfer.Entry, error) {
	infos, err := f.c.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]xfer.Entry, 0, len(infos))
	for _, fi := range infos {
		e := sftpEntry(fi)
		if fi.Mode()&os.ModeSymlink != 0 {
			e.IsLink = true
			if st, err := f.c.Stat(pathpkg.Join(dir, fi.Name())); err == nil {
				e.IsDir, e.Size = st.IsDir(), st.Size()
			}
		}
		out = append(out, e)
	}
	return out, nil
}

func (f sftpFS) Stat(p string) (xfer.Entry, error) {
	fi, err := f.c.Stat(p)
	if err != nil {
		return xfer.Entry{}, err
	}
	e := sftpEntry(fi)
	e.Name = pathpkg.Base(p)
	return e, nil
}

func (f sftpFS) Open(p string, offset int64) (io.ReadCloser, error) {
	file, err := f.c.Open(p)
	if err != nil {
		return nil, err
	}
	if offset > 0 {
		if _, err := file.Seek(offset, io.SeekStart); err != nil {
			file.Close()
			return nil, err
		}
	}
	return file, nil
}

func (f sftpFS) Create(p string, offset int64) (io.WriteCloser, error) {
	flags := os.O_WRONLY | os.O_CREATE
	if offset == 0 {
		flags |= os.O_TRUNC
	}
	file, err := f.c.OpenFile(p, flags)
	if err != nil {
		return nil, err
	}
	if offset > 0 {
		if _, err := file.Seek(offset, io.SeekStart); err != nil {
			file.Close()
			return nil, err
		}
	}
	return file, nil
}

func (f sftpFS) Mkdir(p string) error     { return f.c.Mkdir(p) }
func (f sftpFS) Remove(p string) error    { return f.c.Remove(p) }
func (f sftpFS) RemoveDir(p string) error { return f.c.RemoveDirectory(p) }
func (f sftpFS) Rename(from, to string) error {
	if err := f.c.PosixRename(from, to); err == nil {
		return nil
	}
	return f.c.Rename(from, to)
}
func (f sftpFS) Join(elem ...string) string { return pathpkg.Join(elem...) }

// Lstat não segue links simbólicos.
func (f sftpFS) Lstat(p string) (xfer.Entry, error) {
	fi, err := f.c.Lstat(p)
	if err != nil {
		return xfer.Entry{}, err
	}
	e := sftpEntry(fi)
	e.Name = pathpkg.Base(p)
	e.IsLink = fi.Mode()&os.ModeSymlink != 0
	return e, nil
}

// Real resolve "." e caminhos relativos no servidor.
func (f sftpFS) Real(p string) (string, error) { return f.c.RealPath(p) }
