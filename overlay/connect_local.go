package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"nxaterm/internal/store"

	pty "github.com/aymanbagabas/go-pty"
)

// splitCommand separa "programa arg1 arg2" respeitando aspas duplas, para
// aceitar caminhos com espaço, ex: "C:\Program Files\Git\bin\bash.exe" --login -i
func splitCommand(line string) []string {
	var out []string
	var cur strings.Builder
	inQuote, has := false, false
	for _, r := range line {
		switch {
		case r == '"':
			inQuote = !inQuote
			has = true
		case (r == ' ' || r == '\t') && !inQuote:
			if has {
				out = append(out, cur.String())
				cur.Reset()
				has = false
			}
		default:
			cur.WriteRune(r)
			has = true
		}
	}
	if has {
		out = append(out, cur.String())
	}
	return out
}

// defaultShell é o shell usado quando o perfil não escolhe um.
func defaultShell() string {
	if runtime.GOOS == "windows" {
		return "powershell.exe"
	}
	if sh := os.Getenv("SHELL"); sh != "" {
		return sh
	}
	return "/bin/sh"
}

// startPty roda um programa dentro de um pseudo-terminal (ConPTY no Windows).
func startPty(argv []string, dir string, cols, rows int) (*termSession, error) {
	if len(argv) == 0 {
		return nil, errors.New("comando vazio")
	}
	path, err := exec.LookPath(argv[0])
	if err != nil {
		return nil, fmt.Errorf("programa não encontrado: %s", argv[0])
	}
	p, err := pty.New()
	if err != nil {
		return nil, fmt.Errorf("não foi possível criar o pseudo-terminal: %w", err)
	}
	if err := p.Resize(cols, rows); err != nil {
		p.Close()
		return nil, err
	}
	cmd := p.Command(path, argv[1:]...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	if runtime.GOOS != "windows" {
		cmd.Env = append(cmd.Env, "TERM=xterm-256color")
	}
	if err := cmd.Start(); err != nil {
		p.Close()
		return nil, fmt.Errorf("não foi possível iniciar %s: %w", argv[0], err)
	}
	closePty := sync.OnceFunc(func() { p.Close() })
	t := &termSession{
		w:      p,
		out:    p,
		resize: p.Resize,
		enter:  "\r",
	}
	t.closers = []func(){func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		closePty()
	}}
	t.wait = func() error {
		cmd.Wait() // o código de saída do shell não é um erro do NxATerm
		time.Sleep(150 * time.Millisecond)
		closePty() // no ConPTY a leitura só termina quando o pseudo-console é fechado
		return nil
	}
	return t, nil
}

func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return ""
}

// startShell abre um shell do computador (PowerShell, cmd, bash...).
func startShell(s store.Session, cols, rows int) (*termSession, error) {
	argv := splitCommand(s.Shell)
	if len(argv) == 0 {
		argv = []string{defaultShell()}
	}
	return startPty(argv, homeDir(), cols, rows)
}

// startWSL abre a distribuição Linux do WSL, começando na pasta pessoal dela.
func startWSL(s store.Session, cols, rows int) (*termSession, error) {
	if runtime.GOOS != "windows" {
		return nil, errors.New("o WSL só existe no Windows")
	}
	argv := []string{"wsl.exe"}
	if s.Distro != "" {
		argv = append(argv, "-d", s.Distro)
	}
	argv = append(argv, "--cd", "~")
	return startPty(argv, homeDir(), cols, rows)
}

// userDistros tira da lista as distros internas que Docker Desktop e Rancher Desktop
// criam: elas são mínimas, feitas só para o motor de contêineres, e não servem de terminal.
func userDistros(all []string) []string {
	out := []string{}
	for _, d := range all {
		switch strings.ToLower(d) {
		case "docker-desktop", "docker-desktop-data", "rancher-desktop", "rancher-desktop-data":
			continue
		}
		out = append(out, d)
	}
	return out
}

// WSLDistros lista as distribuições Linux instaladas no WSL (vazio fora do Windows).
func (a *TermService) WSLDistros() []string {
	if runtime.GOOS != "windows" {
		return []string{}
	}
	return userDistros(wslDistros())
}

// LocalShells lista os shells encontrados no computador, para o menu da janela de sessão.
func (a *TermService) LocalShells() []string {
	var cands []string
	if runtime.GOOS == "windows" {
		cands = []string{"powershell.exe", "pwsh.exe", "cmd.exe"}
		for _, git := range []string{
			filepath.Join(os.Getenv("ProgramFiles"), "Git", "bin", "bash.exe"),
			filepath.Join(os.Getenv("ProgramFiles(x86)"), "Git", "bin", "bash.exe"),
		} {
			if st, err := os.Stat(git); err == nil && !st.IsDir() {
				cands = append(cands, `"`+git+`" --login -i`)
				break
			}
		}
	} else {
		cands = []string{defaultShell(), "bash", "zsh", "fish", "sh"}
	}
	var out []string
	seen := map[string]bool{}
	for _, c := range cands {
		argv := splitCommand(c)
		if len(argv) == 0 || seen[c] {
			continue
		}
		if _, err := exec.LookPath(argv[0]); err == nil {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}
