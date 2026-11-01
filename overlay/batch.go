package main

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"nxaterm/internal/store"

	"golang.org/x/crypto/ssh"
)

const maxExecOutput = 512 << 10

// ExecResult é o resultado de um comando executado em lote.
type ExecResult struct {
	ExitCode int    `json:"exitCode"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Error    string `json:"error"` // falha de conexão ou de execução (vazio = rodou)
	Millis   int64  `json:"millis"`
}

// capBuf guarda só o começo da saída, para um comando barulhento não encher a memória.
type capBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
	cut bool
}

func (c *capBuf) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	room := maxExecOutput - c.buf.Len()
	if room > 0 {
		if len(p) > room {
			c.buf.Write(p[:room])
			c.cut = true
		} else {
			c.buf.Write(p)
		}
	} else {
		c.cut = true
	}
	return len(p), nil
}

func (c *capBuf) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := strings.ToValidUTF8(c.buf.String(), "�")
	if c.cut {
		s += "\n[saída cortada]"
	}
	return s
}

// ExecCommand conecta por SSH, roda o comando sem terminal e devolve a saída.
// Falhas de conexão voltam em Error (não como erro Go), para o lote seguir com os outros servidores.
func (a *TermService) ExecCommand(s store.Session, secret, command string, timeoutSec int) ExecResult {
	start := time.Now()
	res := func(r ExecResult) ExecResult { r.Millis = time.Since(start).Milliseconds(); return r }
	if strings.TrimSpace(command) == "" {
		return res(ExecResult{ExitCode: -1, Error: "comando vazio"})
	}
	s, err := store.Normalize(s)
	if err != nil {
		return res(ExecResult{ExitCode: -1, Error: err.Error()})
	}
	if s.Type != store.TypeSSH && s.Type != store.TypeSFTP {
		return res(ExecResult{ExitCode: -1, Error: "só sessões SSH rodam comandos em lote"})
	}
	if timeoutSec <= 0 || timeoutSec > 3600 {
		timeoutSec = 60
	}
	client, err := a.connectSSH(s, secret, 0)
	if err != nil {
		return res(ExecResult{ExitCode: -1, Error: cleanExecErr(err)})
	}
	defer client.Close()
	sess, err := client.NewSession()
	if err != nil {
		return res(ExecResult{ExitCode: -1, Error: err.Error()})
	}
	defer sess.Close()
	for _, kv := range store.EnvPairs(s.Env) {
		sess.Setenv(kv[0], kv[1])
	}
	var out, errb capBuf
	sess.Stdout, sess.Stderr = &out, &errb

	timer := time.AfterFunc(time.Duration(timeoutSec)*time.Second, func() { client.Close() })
	defer timer.Stop()
	runErr := sess.Run(command)

	r := ExecResult{Stdout: out.String(), Stderr: errb.String()}
	var exit *ssh.ExitError
	switch {
	case runErr == nil:
	case errors.As(runErr, &exit):
		r.ExitCode = exit.ExitStatus()
	default:
		r.ExitCode = -1
		if time.Since(start) >= time.Duration(timeoutSec)*time.Second {
			r.Error = fmt.Sprintf("tempo esgotado (%d s)", timeoutSec)
		} else {
			r.Error = cleanExecErr(runErr)
		}
	}
	return res(r)
}

func cleanExecErr(err error) string {
	msg := err.Error()
	if strings.Contains(msg, errPassphraseRequired) {
		return errPassphraseRequired
	}
	return msg
}
