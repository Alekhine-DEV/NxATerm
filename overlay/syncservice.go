package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"nxaterm/internal/store"
	"nxaterm/internal/syncer"
)

// SyncSummary é o resultado de um envio ou de uma busca.
type SyncSummary struct {
	GistID    string `json:"gistId"`
	Sessions  int    `json:"sessions"`  // enviadas, ou adicionadas/mantidas na busca
	Macros    int    `json:"macros"`    //
	Settings  string `json:"settings"`  // configurações da cópia (JSON) para a interface aplicar
	UpdatedAt string `json:"updatedAt"` // quando a cópia foi gravada
	Empty     bool   `json:"empty"`     // não havia cópia no destino
}

func syncCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 60*time.Second)
}

// SyncPush envia sessões, pastas, macros e as configurações indicadas (JSON da interface) para o destino.
func (a *TermService) SyncPush(cfg syncer.Config, settingsJSON string) (SyncSummary, error) {
	a.syncMu.Lock()
	defer a.syncMu.Unlock()
	if settingsJSON == "" {
		settingsJSON = "{}"
	}
	if len(settingsJSON) > 256*1024 || !json.Valid([]byte(settingsJSON)) {
		return SyncSummary{}, errors.New("configurações inválidas")
	}
	sessions := a.store.List()
	macros := a.macros.List()
	var b syncer.Bundle
	var err error
	if b.Sessions, err = json.Marshal(sessions); err != nil {
		return SyncSummary{}, err
	}
	if b.Folders, err = json.Marshal(a.store.Folders()); err != nil {
		return SyncSummary{}, err
	}
	if b.Macros, err = json.Marshal(macros); err != nil {
		return SyncSummary{}, err
	}
	b.Settings = json.RawMessage(settingsJSON)
	ctx, cancel := syncCtx()
	defer cancel()
	id, err := syncer.NewClient().Push(ctx, cfg, b)
	if err != nil {
		return SyncSummary{}, err
	}
	return SyncSummary{GistID: id, Sessions: len(sessions), Macros: len(macros)}, nil
}

// SyncPull baixa a cópia. mode "merge" acrescenta o que falta (o que já existe aqui fica como está);
// "replace" troca tudo pelo que veio.
func (a *TermService) SyncPull(cfg syncer.Config, mode string) (SyncSummary, error) {
	a.syncMu.Lock()
	defer a.syncMu.Unlock()
	if mode != "merge" && mode != "replace" {
		return SyncSummary{}, errors.New("modo de sincronização inválido")
	}
	ctx, cancel := syncCtx()
	defer cancel()
	b, err := syncer.NewClient().Pull(ctx, cfg)
	if errors.Is(err, syncer.ErrNotFound) {
		return SyncSummary{Empty: true}, nil
	}
	if err != nil {
		return SyncSummary{}, err
	}
	var sessions []store.Session
	var macros []store.Macro
	var folders []string
	if len(b.Sessions) > 0 {
		if err := json.Unmarshal(b.Sessions, &sessions); err != nil {
			return SyncSummary{}, errors.New("sessões da cópia inválidas")
		}
	}
	if len(b.Macros) > 0 {
		if err := json.Unmarshal(b.Macros, &macros); err != nil {
			return SyncSummary{}, errors.New("macros da cópia inválidas")
		}
	}
	if len(b.Folders) > 0 {
		if err := json.Unmarshal(b.Folders, &folders); err != nil {
			return SyncSummary{}, errors.New("pastas da cópia inválidas")
		}
	}
	sum := SyncSummary{Settings: string(b.Settings), UpdatedAt: b.UpdatedAt}
	if mode == "replace" {
		if sum.Sessions, err = a.store.ReplaceAll(sessions, folders); err != nil {
			return sum, err
		}
		if sum.Macros, err = a.macros.ReplaceAll(macros); err != nil {
			return sum, err
		}
		return sum, nil
	}
	if sum.Sessions, err = a.store.MergeSync(sessions, folders); err != nil {
		return sum, err
	}
	sum.Macros, err = a.macros.MergeSync(macros)
	return sum, err
}

// ---------------------------------------------------------------- credenciais (opcional)

func (a *TermService) syncSecretsPath() (string, error) {
	dir, err := dataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sync-secrets.json"), nil
}

// SyncSaveSecrets guarda no disco (só para o usuário, modo 0600) o token/senhas do destino.
// É opcional: sem isso, a interface guarda os dados só na memória e pede de novo ao reabrir.
func (a *TermService) SyncSaveSecrets(cfg syncer.Config) error {
	p, err := a.syncSecretsPath()
	if err != nil {
		return err
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

// SyncLoadSecrets devolve o que foi guardado (vazio se nada).
func (a *TermService) SyncLoadSecrets() (syncer.Config, error) {
	var cfg syncer.Config
	p, err := a.syncSecretsPath()
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	err = json.Unmarshal(data, &cfg)
	return cfg, err
}

// SyncForgetSecrets apaga o que foi guardado.
func (a *TermService) SyncForgetSecrets() error {
	p, err := a.syncSecretsPath()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
