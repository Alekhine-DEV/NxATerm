package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"nxaterm/internal/secfile"
	"nxaterm/internal/store"
	"nxaterm/internal/tunnel"
)

// TunnelDef é um túnel salvo (a lista da janela "Túneis SSH"). Cada túnel abre a sua própria
// conexão SSH, sem depender de uma aba aberta. A senha nunca é guardada.
type TunnelDef struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"` // "local", "remote" ou "dynamic"
	ListenPort int    `json:"listenPort"`
	DestHost   string `json:"destHost"`
	DestPort   int    `json:"destPort"`
	SessionID  string `json:"sessionId"` // sessão SSH salva usada como base (proxy e servidor de salto)
	Host       string `json:"host"`
	Port       int    `json:"port"`
	User       string `json:"user"`
	AuthMethod string `json:"authMethod"` // "password" ou "key"
	KeyPath    string `json:"keyPath"`
}

// TunnelInfo descreve um túnel ativo.
type TunnelInfo struct {
	ID         string `json:"id"`
	DefID      string `json:"defId"`
	Kind       string `json:"kind"`
	ListenPort int    `json:"listenPort"`
	DestHost   string `json:"destHost"`
	DestPort   int    `json:"destPort"`
	Desc       string `json:"desc"`
}

type activeTunnel struct {
	info   TunnelInfo
	tn     *tunnel.Tunnel
	client *ssh.Client
	once   sync.Once
}

func (t *activeTunnel) stop() {
	t.once.Do(func() {
		t.tn.Stop()
		t.client.Close()
	})
}

func (a *TermService) tunnelsPath() string {
	return filepath.Join(filepath.Dir(a.settingsPath), "tunnels.json")
}

// LoadTunnelDefs devolve o JSON da lista de túneis salvos ("" se ainda não existe).
func (a *TermService) LoadTunnelDefs() (string, error) {
	data, err := secfile.Read(a.tunnelsPath())
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return string(data), err
}

// SaveTunnelDefs grava a lista de túneis salvos (criptografada quando há senha de abertura).
func (a *TermService) SaveTunnelDefs(js string) error {
	if len(js) > 1<<20 {
		return errors.New("lista de túneis grande demais")
	}
	return secfile.Write(a.tunnelsPath(), []byte(js))
}

// tunnelSession monta a sessão SSH do túnel: parte da sessão salva escolhida (para herdar proxy e
// servidor de salto) e aplica por cima o que está no próprio túnel.
func (a *TermService) tunnelSession(d TunnelDef) (store.Session, error) {
	var s store.Session
	if d.SessionID != "" {
		if b, ok := a.store.Get(d.SessionID); ok && (b.Type == store.TypeSSH || b.Type == store.TypeSFTP) {
			s = b
		}
	}
	s.Type = store.TypeSSH
	if d.Host != "" {
		s.Host = d.Host
	}
	if d.Port != 0 {
		s.Port = d.Port
	}
	if d.User != "" {
		s.User = d.User
	}
	if d.AuthMethod != "" {
		s.AuthMethod = d.AuthMethod
	}
	if d.AuthMethod == "key" || s.AuthMethod == "key" {
		s.KeyPath = d.KeyPath
	}
	if s.Name == "" {
		s.Name = d.Name
	}
	if s.Name == "" {
		s.Name = s.Host
	}
	if s.Host == "" {
		return s, errors.New("informe o servidor SSH")
	}
	if s.Port == 0 {
		s.Port = 22
	}
	return store.Normalize(s)
}

// TunnelOpen conecta ao servidor SSH do túnel e o coloca para funcionar. secret é a senha (ou a
// senha da chave privada).
//
//	local   : 127.0.0.1:listenPort (aqui)  ->  destHost:destPort (visto do servidor)
//	remote  : 127.0.0.1:listenPort (servidor) -> destHost:destPort (visto daqui)
//	dynamic : proxy SOCKS5 em 127.0.0.1:listenPort (aqui), saindo pelo servidor
//
// Os túneis só escutam em 127.0.0.1. Evento "tunnel:closed" (com o ID do túnel salvo) quando a
// conexão cai.
func (a *TermService) TunnelOpen(d TunnelDef, secret string) (TunnelInfo, error) {
	if d.ID == "" {
		return TunnelInfo{}, errors.New("túnel sem identificador")
	}
	a.tunMu.Lock()
	for _, t := range a.tunnels {
		if t.info.DefID == d.ID {
			a.tunMu.Unlock()
			return TunnelInfo{}, errors.New("este túnel já está ativo")
		}
	}
	a.tunMu.Unlock()

	s, err := a.tunnelSession(d)
	if err != nil {
		return TunnelInfo{}, err
	}
	client, err := a.connectSSH(s, secret, 0)
	if err != nil {
		return TunnelInfo{}, err
	}
	tn, err := tunnel.Start(client, d.Kind, d.ListenPort, d.DestHost, d.DestPort)
	if err != nil {
		client.Close()
		return TunnelInfo{}, err
	}
	b := make([]byte, 6)
	rand.Read(b)
	at := &activeTunnel{tn: tn, client: client, info: TunnelInfo{
		ID: hex.EncodeToString(b), DefID: d.ID, Kind: d.Kind,
		ListenPort: d.ListenPort, DestHost: d.DestHost, DestPort: d.DestPort, Desc: tn.Desc,
	}}
	a.tunMu.Lock()
	a.tunnels[at.info.ID] = at
	a.tunMu.Unlock()

	// conexão caiu: o túnel acaba e a janela é avisada
	go func() {
		client.Wait()
		a.tunMu.Lock()
		_, still := a.tunnels[at.info.ID]
		delete(a.tunnels, at.info.ID)
		a.tunMu.Unlock()
		at.stop()
		if still {
			a.app.Event.Emit("tunnel:closed", at.info.DefID)
		}
	}()
	// keepalive para não cair por inatividade
	go func() {
		tk := time.NewTicker(30 * time.Second)
		defer tk.Stop()
		for range tk.C {
			if _, _, err := client.SendRequest("keepalive@openssh.com", true, nil); err != nil {
				return
			}
		}
	}()
	return at.info, nil
}

// TunnelList devolve os túneis ativos.
func (a *TermService) TunnelList() []TunnelInfo {
	a.tunMu.Lock()
	defer a.tunMu.Unlock()
	out := make([]TunnelInfo, 0, len(a.tunnels))
	for _, t := range a.tunnels {
		out = append(out, t.info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ListenPort < out[j].ListenPort })
	return out
}

// TunnelStop encerra um túnel ativo.
func (a *TermService) TunnelStop(id string) error {
	a.tunMu.Lock()
	t, ok := a.tunnels[id]
	delete(a.tunnels, id)
	a.tunMu.Unlock()
	if !ok {
		return errors.New("túnel não encontrado")
	}
	t.stop()
	return nil
}

func (a *TermService) stopAllTunnels() {
	a.tunMu.Lock()
	list := make([]*activeTunnel, 0, len(a.tunnels))
	for id, t := range a.tunnels {
		list = append(list, t)
		delete(a.tunnels, id)
	}
	a.tunMu.Unlock()
	for _, t := range list {
		t.stop()
	}
}
