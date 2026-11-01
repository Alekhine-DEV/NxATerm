package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Arquivos abertos durante uma transferência Zmodem (rz/sz). O protocolo em si
// roda no frontend (zmodem.js); aqui ficam só os bytes e o disco.

const zmChunkMax = 1 << 20

type zmFile struct {
	f    *os.File
	path string
	recv bool // arquivo que está sendo recebido (apagado se a transferência falhar)
}

var zm = struct {
	mu    sync.Mutex
	files map[string]*zmFile
	seq   int
}{files: map[string]*zmFile{}}

func zmAdd(f *zmFile) string {
	zm.mu.Lock()
	defer zm.mu.Unlock()
	zm.seq++
	id := "z" + strconv.Itoa(zm.seq)
	zm.files[id] = f
	return id
}

func zmGet(id string) (*zmFile, error) {
	zm.mu.Lock()
	defer zm.mu.Unlock()
	f, ok := zm.files[id]
	if !ok {
		return nil, errors.New("arquivo Zmodem não está aberto")
	}
	return f, nil
}

// closeAllZmodem fecha tudo ao encerrar o app.
func closeAllZmodem() {
	zm.mu.Lock()
	defer zm.mu.Unlock()
	for id, f := range zm.files {
		f.f.Close()
		if f.recv {
			os.Remove(f.path)
		}
		delete(zm.files, id)
	}
}

// WriteBytes envia bytes brutos (base64) para a sessão. O Zmodem precisa disso:
// Write só aceita texto.
func (a *TermService) WriteBytes(id, b64 string) error {
	t, err := a.term(id)
	if err != nil {
		return err
	}
	if t.w == nil {
		return errors.New("esta aba não tem terminal")
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return err
	}
	_, err = t.w.Write(data)
	return err
}

// ZFileInfo descreve um arquivo escolhido para enviar.
type ZFileInfo struct {
	Handle string `json:"handle"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Mtime  int64  `json:"mtime"` // unix, segundos
	Mode   int    `json:"mode"`
}

// ZPickSend pergunta quais arquivos enviar e os deixa abertos para leitura.
// Lista vazia = o usuário cancelou.
func (a *TermService) ZPickSend() ([]ZFileInfo, error) {
	paths, err := a.app.Dialog.OpenFile().SetTitle("Escolha os arquivos para enviar (Zmodem)").PromptForMultipleSelection()
	if err != nil && !isCancel(err) {
		return nil, err
	}
	if err != nil || len(paths) == 0 {
		return nil, nil
	}
	var out []ZFileInfo
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil || st.IsDir() {
			continue
		}
		f, err := os.Open(p)
		if err != nil {
			return out, fmt.Errorf("%s: %w", filepath.Base(p), err)
		}
		out = append(out, ZFileInfo{
			Handle: zmAdd(&zmFile{f: f, path: p}),
			Name:   filepath.Base(p), Size: st.Size(), Mtime: st.ModTime().Unix(), Mode: int(st.Mode().Perm()),
		})
	}
	return out, nil
}

// ZRead lê até n bytes do arquivo (base64). Texto vazio = fim do arquivo.
func (a *TermService) ZRead(handle string, n int) (string, error) {
	f, err := zmGet(handle)
	if err != nil {
		return "", err
	}
	if n <= 0 || n > zmChunkMax {
		n = zmChunkMax
	}
	buf := make([]byte, n)
	got, err := io.ReadFull(f.f, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf[:got]), nil
}

// ZPickDir pergunta em que pasta salvar os arquivos recebidos ("" = cancelou).
func (a *TermService) ZPickDir() (string, error) {
	p, err := a.app.Dialog.OpenFile().SetTitle("Salvar arquivos recebidos em (Zmodem)").
		CanChooseDirectories(true).CanChooseFiles(false).CanCreateDirectories(true).PromptForSingleSelection()
	if err != nil && !isCancel(err) {
		return "", err
	}
	if err != nil {
		return "", nil
	}
	return p, nil
}

// zmSafeName deixa só o nome do arquivo: o servidor controla esse texto e não pode
// escolher pastas nem sobrescrever nada fora da pasta escolhida.
func zmSafeName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = name[strings.LastIndex(name, "/")+1:]
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 0x7f || strings.ContainsRune(`<>:"|?*`, r) {
			return '_'
		}
		return r
	}, name)
	name = strings.TrimSpace(strings.TrimRight(name, ". "))
	if name == "" || name == "." || name == ".." {
		return "arquivo"
	}
	return name
}

// ZCreate cria o arquivo recebido dentro de dir, sem sobrescrever (acrescenta " (2)", etc.).
func (a *TermService) ZCreate(dir, name string) (ZFileInfo, error) {
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return ZFileInfo{}, errors.New("pasta de destino inválida")
	}
	base := zmSafeName(name)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for i := 1; i < 10000; i++ {
		n := base
		if i > 1 {
			n = fmt.Sprintf("%s (%d)%s", stem, i, ext)
		}
		p := filepath.Join(dir, n)
		f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			if os.IsExist(err) {
				continue
			}
			return ZFileInfo{}, err
		}
		return ZFileInfo{Handle: zmAdd(&zmFile{f: f, path: p, recv: true}), Name: n}, nil
	}
	return ZFileInfo{}, errors.New("não foi possível escolher um nome livre")
}

// ZWrite grava um pedaço (base64) no arquivo recebido.
func (a *TermService) ZWrite(handle, b64 string) error {
	f, err := zmGet(handle)
	if err != nil {
		return err
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return err
	}
	_, err = f.f.Write(data)
	return err
}

// ZClose fecha o arquivo. Num recebimento, ok=false apaga o que ficou pela metade;
// ok=true ajusta a data de modificação (mtime, unix; 0 = deixa como está).
func (a *TermService) ZClose(handle string, ok bool, mtime int64) error {
	zm.mu.Lock()
	f, found := zm.files[handle]
	delete(zm.files, handle)
	zm.mu.Unlock()
	if !found {
		return nil
	}
	err := f.f.Close()
	if f.recv {
		if !ok {
			os.Remove(f.path)
		} else if mtime > 0 {
			t := time.Unix(mtime, 0)
			os.Chtimes(f.path, t, t)
		}
	}
	return err
}
