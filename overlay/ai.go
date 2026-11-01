package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"nxaterm/internal/aichat"
	"nxaterm/internal/secfile"
)

// agentTools são as ferramentas que o modo agente oferece ao modelo.
var agentTools = []aichat.Tool{{
	Name:        "run_command",
	Description: "Executa um comando de shell no servidor (ou no computador) da sessão ativa e devolve a saída. O usuário aprova cada comando antes de rodar. Prefira comandos curtos, não interativos e que não alterem nada sem necessidade.",
	Parameters:  json.RawMessage(`{"type":"object","properties":{"command":{"type":"string","description":"O comando a executar"},"reason":{"type":"string","description":"Em uma frase, por que este comando é necessário"}},"required":["command"]}`),
}}

// AiChat envia a conversa ao modelo. Com agent=true, o modelo pode pedir o run_command.
func (a *TermService) AiChat(cfg aichat.Config, msgs []aichat.Message, agent bool) (aichat.Reply, error) {
	if len(msgs) > 400 {
		msgs = msgs[len(msgs)-400:]
	}
	var tools []aichat.Tool
	if agent {
		tools = agentTools
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	return aichat.NewClient().Chat(ctx, cfg, msgs, tools)
}

// AiModels lista os modelos do serviço configurado.
func (a *TermService) AiModels(cfg aichat.Config) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return aichat.NewClient().Models(ctx, cfg)
}

func (a *TermService) aiKeyPath() (string, error) {
	dir, err := dataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ai-key.txt"), nil
}

// AiSaveKey guarda a chave da API num arquivo só do usuário (opcional; sem isso fica só na memória).
func (a *TermService) AiSaveKey(key string) error {
	p, err := a.aiKeyPath()
	if err != nil {
		return err
	}
	return secfile.Write(p, []byte(strings.TrimSpace(key)))
}

// AiLoadKey devolve a chave guardada (vazio se nada).
func (a *TermService) AiLoadKey() (string, error) {
	p, err := a.aiKeyPath()
	if err != nil {
		return "", err
	}
	data, err := secfile.Read(p)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return strings.TrimSpace(string(data)), err
}

// AiForgetKey apaga a chave guardada.
func (a *TermService) AiForgetKey() error {
	p, err := a.aiKeyPath()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// ExecLocal roda um comando no computador do usuário (para o agente, quando a aba ativa é um shell local).
func (a *TermService) ExecLocal(command string, timeoutSec int) ExecResult {
	start := time.Now()
	res := func(r ExecResult) ExecResult { r.Millis = time.Since(start).Milliseconds(); return r }
	if strings.TrimSpace(command) == "" {
		return res(ExecResult{ExitCode: -1, Error: "comando vazio"})
	}
	if timeoutSec <= 0 || timeoutSec > 600 {
		timeoutSec = 60
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", command)
		hideWindow(cmd)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", command)
	}
	var out, errb capBuf
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	r := ExecResult{Stdout: out.String(), Stderr: errb.String()}
	var ee *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &ee):
		r.ExitCode = ee.ExitCode()
	case ctx.Err() != nil:
		r.ExitCode = -1
		r.Error = "tempo esgotado"
	default:
		r.ExitCode = -1
		r.Error = err.Error()
	}
	return res(r)
}
