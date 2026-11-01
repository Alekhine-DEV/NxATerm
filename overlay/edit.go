package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nxaterm/internal/xfer"
)

const maxEditSize = 50 << 20

// setting lê uma chave de texto do settings.json.
func (a *TermService) setting(key string) string {
	data, err := os.ReadFile(a.settingsPath)
	if err != nil {
		return ""
	}
	var m map[string]any
	if json.Unmarshal(data, &m) != nil {
		return ""
	}
	s, _ := m[key].(string)
	return strings.TrimSpace(s)
}

type editSaved struct {
	TabID string `json:"tabId"`
	Path  string `json:"path"`
	Name  string `json:"name"`
	OK    bool   `json:"ok"`
	Err   string `json:"err"`
}

// EditRemote baixa o arquivo remoto para uma pasta temporária, abre no editor
// (o configurado ou o programa padrão do sistema) e envia de volta toda vez que
// o arquivo for salvo. A pasta temporária some quando a aba fecha.
func (a *TermService) EditRemote(tabID, remotePath string) error {
	t, err := a.term(tabID)
	if err != nil {
		return err
	}
	rfs, err := t.files()
	if err != nil {
		return err
	}
	e, err := rfs.Stat(remotePath)
	if err != nil {
		return err
	}
	if e.IsDir {
		return errors.New("selecione um arquivo")
	}
	if e.Size > maxEditSize {
		return errors.New("arquivo grande demais para editar (limite de 50 MB)")
	}
	dir, err := os.MkdirTemp("", "nxaterm-edit-")
	if err != nil {
		return err
	}
	name := baseName(remotePath)
	local := filepath.Join(dir, name)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if _, err := xfer.Copy(ctx, rfs, xfer.Local{}, remotePath, local, xfer.Options{Policy: xfer.Overwrite}); err != nil {
		os.RemoveAll(dir)
		return err
	}
	fi, err := os.Stat(local)
	if err != nil {
		os.RemoveAll(dir)
		return err
	}

	if err := a.openInEditor(local); err != nil {
		os.RemoveAll(dir)
		return err
	}
	go a.watchEdit(t, rfs, tabID, remotePath, local, dir, fi.ModTime(), fi.Size())
	return nil
}

func (a *TermService) openInEditor(local string) error {
	ed := a.setting("editor")
	if ed == "" {
		return openPath(local)
	}
	if _, err := os.Stat(ed); err == nil {
		return runDetached(ed, local)
	}
	f := strings.Fields(ed)
	return runDetached(f[0], append(f[1:], local)...)
}

func (a *TermService) watchEdit(t *termSession, rfs xfer.FS, tabID, remote, local, dir string, mod time.Time, size int64) {
	defer os.RemoveAll(dir)
	name := baseName(remote)
	deadline := time.After(12 * time.Hour)
	tk := time.NewTicker(time.Second)
	defer tk.Stop()
	pending := false
	for {
		select {
		case <-t.done:
			return
		case <-deadline:
			return
		case <-tk.C:
		}
		fi, err := os.Stat(local)
		if err != nil {
			continue
		}
		if !fi.ModTime().Equal(mod) || fi.Size() != size {
			mod, size = fi.ModTime(), fi.Size() // ainda gravando: espera estabilizar
			pending = true
			continue
		}
		if !pending {
			continue
		}
		pending = false
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		_, err = xfer.Copy(ctx, xfer.Local{}, rfs, local, remote, xfer.Options{Policy: xfer.Overwrite})
		cancel()
		ev := editSaved{TabID: tabID, Path: remote, Name: name, OK: err == nil}
		if err != nil {
			ev.Err = fmt.Sprint(err)
		}
		a.emitJSON("edit:saved", ev)
	}
}
