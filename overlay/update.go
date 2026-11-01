package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"

	"nxaterm/internal/appver"
)

// Repositório consultado para ver se há versão nova.
const releaseAPI = "https://api.github.com/repos/Alekhine-DEV/NxATerm/releases/latest"

// AppInfo é o que a janela "Sobre" mostra.
type AppInfo struct {
	Version string `json:"version"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
}

// GetAppInfo devolve a versão do aplicativo e a plataforma.
func (a *TermService) GetAppInfo() AppInfo {
	return AppInfo{Version: appVersion, OS: runtime.GOOS, Arch: runtime.GOARCH}
}

// UpdateInfo é o resultado da verificação de atualização.
type UpdateInfo struct {
	Current string `json:"current"`
	Latest  string `json:"latest"`
	URL     string `json:"url"`
	Notes   string `json:"notes"`
	Newer   bool   `json:"newer"`
}

// CheckUpdate consulta a última versão publicada no GitHub.
func (a *TermService) CheckUpdate() (UpdateInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releaseAPI, nil)
	if err != nil {
		return UpdateInfo{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "NxATerm/"+appVersion)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return UpdateInfo{}, fmt.Errorf("não foi possível consultar as versões: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return UpdateInfo{}, fmt.Errorf("o GitHub respondeu %d", resp.StatusCode)
	}
	var rel struct {
		Tag  string `json:"tag_name"`
		URL  string `json:"html_url"`
		Body string `json:"body"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rel); err != nil {
		return UpdateInfo{}, err
	}
	if rel.Tag == "" {
		return UpdateInfo{}, errors.New("resposta sem versão")
	}
	latest := strings.TrimPrefix(rel.Tag, "v")
	notes := rel.Body
	if len(notes) > 4000 {
		notes = notes[:4000]
	}
	return UpdateInfo{Current: appVersion, Latest: latest, URL: rel.URL, Notes: notes, Newer: appver.Newer(latest, appVersion)}, nil
}
