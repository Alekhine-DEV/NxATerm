package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"nxaterm/internal/secfile"
	"nxaterm/internal/store"

	"golang.org/x/crypto/ssh"
)

// ---------------------------------------------------------------- macros

// ListMacros devolve as macros salvas.
func (a *TermService) ListMacros() []store.Macro { return a.macros.List() }

// SaveMacro cria (id vazio) ou atualiza uma macro.
func (a *TermService) SaveMacro(m store.Macro) (store.Macro, error) { return a.macros.Save(m) }

// ReorderMacros grava a ordem escolhida pelo usuário (IDs na sequência).
func (a *TermService) ReorderMacros(ids []string) error { return a.macros.Reorder(ids) }

// HasLock diz se existe senha de abertura.
func (a *TermService) HasLock() bool { return a.lock.Enabled() }

// CheckLock confere a senha de abertura e, se estiver certa, abre os dados criptografados.
func (a *TermService) CheckLock(pw string) (bool, error) {
	if !a.lock.Check(pw) {
		return false, nil
	}
	if !a.lock.Enabled() {
		return true, nil
	}
	if err := a.unlockData(pw); err != nil {
		return false, err
	}
	return true, nil
}

// SetLock define, troca ou remove (newPw vazia) a senha de abertura. Os arquivos de dados são
// regravados com a chave nova (ou em texto, se a senha for removida).
func (a *TermService) SetLock(current, newPw string) error {
	a.dataMu.Lock()
	defer a.dataMu.Unlock()
	if a.store.Locked() || a.macros.Locked() || a.tombs.Locked() {
		return secfile.ErrLocked
	}
	// o conteúdo atual dos arquivos, ainda com a chave antiga
	files, err := a.readSecretFiles()
	if err != nil {
		return err
	}
	if err := a.lock.Set(current, newPw); err != nil {
		return err
	}
	if newPw == "" {
		secfile.ClearKey()
	} else {
		key, err := a.lock.DeriveKey(newPw)
		if err != nil {
			return err
		}
		secfile.SetKey(key)
	}
	return a.writeAllData(files)
}

// unlockData carrega a chave dos dados a partir da senha (já conferida) e lê os arquivos.
func (a *TermService) unlockData(pw string) error {
	a.dataMu.Lock()
	defer a.dataMu.Unlock()
	if secfile.HasKey() && !a.store.Locked() && !a.macros.Locked() && !a.tombs.Locked() {
		return nil
	}
	key, err := a.lock.DeriveKey(pw)
	if err != nil {
		return err
	}
	secfile.SetKey(key)
	fail := func(err error) error {
		secfile.ClearKey()
		return err
	}
	if err := a.store.Reload(); err != nil {
		return fail(err)
	}
	if err := a.macros.Reload(); err != nil {
		return fail(err)
	}
	if err := a.tombs.Reload(); err != nil {
		return fail(err)
	}
	files, err := a.readSecretFiles()
	if err != nil {
		return fail(err)
	}
	// arquivos de uma versão sem criptografia passam a ficar protegidos
	return a.writeAllData(files)
}

// secretFileNames são os arquivos, além de sessões e macros, que acompanham a criptografia.
var secretFileNames = []string{"settings.json", "tunnels.json", "ai-key.txt", "mcp-token.txt", "sync-secrets.json"}

func (a *TermService) readSecretFiles() (map[string][]byte, error) {
	dir := filepath.Dir(a.settingsPath)
	out := map[string][]byte{}
	for _, n := range secretFileNames {
		data, err := secfile.Read(filepath.Join(dir, n))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", n, err)
		}
		out[n] = data
	}
	return out, nil
}

// writeAllData regrava sessões, macros e os demais arquivos no formato atual (com ou sem chave).
func (a *TermService) writeAllData(files map[string][]byte) error {
	var errs []error
	if err := a.store.Rewrite(); err != nil {
		errs = append(errs, err)
	}
	if err := a.macros.Rewrite(); err != nil {
		errs = append(errs, err)
	}
	if err := a.tombs.Rewrite(); err != nil {
		errs = append(errs, err)
	}
	dir := filepath.Dir(a.settingsPath)
	for n, data := range files {
		if err := secfile.Write(filepath.Join(dir, n), data); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", n, err))
		}
	}
	if js, ok := files["settings.json"]; ok {
		a.writeStartup(js)
	}
	return errors.Join(errs...)
}

// writeStartup guarda em texto só o que precisa valer antes de a senha ser digitada
// (bandeja, atalho global e janela única). Não tem nada sensível.
func (a *TermService) writeStartup(settingsJSON []byte) {
	var st startupSettings
	if json.Unmarshal(settingsJSON, &st) != nil {
		return
	}
	if b, err := json.Marshal(st); err == nil {
		store.WriteFileAtomic(filepath.Join(filepath.Dir(a.settingsPath), "startup.json"), b)
	}
}

// DeleteMacro apaga uma macro.
func (a *TermService) DeleteMacro(id string) error { return a.macros.Delete(id) }

// ---------------------------------------------------------------- configurações

// LoadSettings devolve o JSON das configurações da interface ("" se ainda não existe).
func (a *TermService) LoadSettings() (string, error) {
	data, err := secfile.Read(a.settingsPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return string(data), err
}

// SaveSettings grava o JSON das configurações (o conteúdo é só da interface).
func (a *TermService) SaveSettings(js string) error {
	if len(js) > 256*1024 {
		return errors.New("configurações grandes demais")
	}
	if !json.Valid([]byte(js)) {
		return errors.New("configurações inválidas")
	}
	if err := secfile.Write(a.settingsPath, []byte(js)); err != nil {
		return err
	}
	a.writeStartup([]byte(js))
	return nil
}

// ---------------------------------------------------------------- gerador de chaves

// KeyResult é o resultado de GenerateKey.
type KeyResult struct {
	Path      string `json:"path"`
	PublicKey string `json:"publicKey"`
}

// GenerateKey cria um par de chaves Ed25519 no formato OpenSSH. Pergunta onde
// salvar; nunca sobrescreve um arquivo existente. A chave pública fica em <arquivo>.pub.
// Devolve Path vazio se o usuário cancelar.
func (a *TermService) GenerateKey(comment, passphrase string) (KeyResult, error) {
	path, err := a.app.Dialog.SaveFile().
		SetFilename("id_ed25519").
		PromptForSingleSelection()
	if err != nil && !isCancel(err) {
		return KeyResult{}, err
	}
	if err != nil || path == "" {
		return KeyResult{}, nil
	}
	if _, err := os.Stat(path); err == nil {
		return KeyResult{}, fmt.Errorf("%s já existe; escolha outro nome para não sobrescrever uma chave", path)
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return KeyResult{}, err
	}
	var block *pem.Block
	if passphrase == "" {
		block, err = ssh.MarshalPrivateKey(priv, comment)
	} else {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(priv, comment, []byte(passphrase))
	}
	if err != nil {
		return KeyResult{}, err
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		return KeyResult{}, err
	}
	sp, err := ssh.NewPublicKey(pub)
	if err != nil {
		return KeyResult{}, err
	}
	line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sp)))
	if comment != "" {
		line += " " + comment
	}
	if err := os.WriteFile(path+".pub", []byte(line+"\n"), 0o644); err != nil {
		return KeyResult{}, err
	}
	return KeyResult{Path: path, PublicKey: line}, nil
}

// ---------------------------------------------------------------- app

// Quit fecha o aplicativo.
func (a *TermService) Quit() { a.quit() }

// SetGlobalHotkey registra (ou, com texto vazio, remove) o atalho que mostra/esconde a janela.
// Funciona no Windows, no Linux com X11 e no macOS; no Wayland o sistema não permite.
func (a *TermService) SetGlobalHotkey(spec string) error {
	return setGlobalHotkey(spec, a.toggleWindow)
}

func (a *TermService) toggleWindow() {
	if a.toggle != nil {
		a.toggle()
	}
}
