package xfer

import (
	"io"
	"os"
	"path/filepath"
)

// Local é o disco da máquina do usuário.
type Local struct{}

func entryOf(fi os.FileInfo) Entry {
	return Entry{Name: fi.Name(), Size: fi.Size(), IsDir: fi.IsDir(), Mode: fi.Mode(), ModTime: fi.ModTime()}
}

func (Local) List(dir string) ([]Entry, error) {
	des, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(des))
	for _, de := range des {
		fi, err := de.Info()
		if err != nil {
			continue // sumiu no meio da listagem
		}
		e := entryOf(fi)
		if fi.Mode()&os.ModeSymlink != 0 {
			e.IsLink = true
			if st, err := os.Stat(filepath.Join(dir, de.Name())); err == nil {
				e.IsDir, e.Size = st.IsDir(), st.Size()
			}
		}
		out = append(out, e)
	}
	return out, nil
}

func (Local) Stat(p string) (Entry, error) {
	fi, err := os.Stat(p)
	if err != nil {
		return Entry{}, err
	}
	return entryOf(fi), nil
}

func (Local) Open(p string, offset int64) (io.ReadCloser, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			f.Close()
			return nil, err
		}
	}
	return f, nil
}

func (Local) Create(p string, offset int64) (io.WriteCloser, error) {
	flags := os.O_WRONLY | os.O_CREATE
	if offset == 0 {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(p, flags, 0o644)
	if err != nil {
		return nil, err
	}
	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			f.Close()
			return nil, err
		}
	}
	return f, nil
}

func (Local) Mkdir(p string) error         { return os.Mkdir(p, 0o755) }
func (Local) Remove(p string) error        { return os.Remove(p) }
func (Local) RemoveDir(p string) error     { return os.Remove(p) }
func (Local) Rename(from, to string) error { return os.Rename(from, to) }
func (Local) Join(elem ...string) string   { return filepath.Join(elem...) }
