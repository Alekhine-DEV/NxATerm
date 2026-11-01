package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strings"

	"nxaterm/internal/store"

	"golang.org/x/crypto/ssh"
)

// ---------------------------------------------------------------- macros

// ListMacros devolve as macros salvas.
func (a *TermService) ListMacros() []store.Macro { return a.macros.List() }

// SaveMacro cria (id vazio) ou atualiza uma macro.
func (a *TermService) SaveMacro(m store.Macro) (store.Macro, error) { return a.macros.Save(m) }

// DeleteMacro apaga uma macro.
func (a *TermService) DeleteMacro(id string) error { return a.macros.Delete(id) }

// ---------------------------------------------------------------- configurações

// LoadSettings devolve o JSON das configurações da interface ("" se ainda não existe).
func (a *TermService) LoadSettings() (string, error) {
	data, err := os.ReadFile(a.settingsPath)
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
	return store.WriteFileAtomic(a.settingsPath, []byte(js))
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
func (a *TermService) Quit() { a.app.Quit() }
