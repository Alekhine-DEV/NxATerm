package store

import (
	"bufio"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// decodeLegacy converte o texto de um arquivo do MobaXterm (Windows-1252) para UTF-8.
// Se o arquivo já é UTF-8 válido, devolve como está.
func decodeLegacy(b []byte) string {
	if utf8.Valid(b) {
		return strings.TrimPrefix(string(b), "\ufeff")
	}
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c) // Latin-1 cobre os acentos do português
	}
	return string(r)
}

// MobaResult é o que ParseMobaXterm encontrou.
type MobaResult struct {
	Sessions []Session
	Folders  []string
	Skipped  int // sessões de tipos que o NxATerm não importa (RDP, VNC, FTP...)
}

// ParseMobaXterm lê um arquivo .mxtsessions ou o MobaXterm.ini. Cada seção
// [Bookmarks...] é uma pasta (SubRep) e cada linha "Nome=#ícone#tipo%host%porta%usuário%..."
// é uma sessão. Importa SSH, SFTP e Telnet; chaves .ppk não são lidas pelo NxATerm,
// então essas sessões entram com senha e a chave original vai para as notas.
func ParseMobaXterm(r io.Reader) (MobaResult, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return MobaResult{}, err
	}
	var res MobaResult
	folder := ""
	inBookmarks := false
	sc := bufio.NewScanner(strings.NewReader(decodeLegacy(raw)))
	sc.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			inBookmarks = strings.HasPrefix(strings.ToLower(line), "[bookmarks")
			folder = ""
			continue
		}
		if !inBookmarks {
			continue
		}
		eq := strings.Index(line, "=")
		if eq < 0 {
			continue
		}
		name, val := strings.TrimSpace(line[:eq]), strings.TrimSpace(line[eq+1:])
		switch strings.ToLower(name) {
		case "subrep":
			folder = CleanFolder(val)
			if folder != "" {
				res.Folders = append(res.Folders, folder)
			}
			continue
		case "imgnum":
			continue
		}
		s, ok, skipped := mobaSession(name, val, folder)
		switch {
		case ok:
			res.Sessions = append(res.Sessions, s)
		case skipped:
			res.Skipped++
		}
	}
	return res, sc.Err()
}

// mobaSession interpreta uma linha de sessão. skipped=true quando é de um tipo não suportado.
func mobaSession(name, val, folder string) (s Session, ok, skipped bool) {
	val = strings.TrimSpace(strings.TrimPrefix(val, ";  logout")) // prefixo de "sem mensagem ao reconectar"
	if !strings.HasPrefix(val, "#") {
		return s, false, false
	}
	parts := strings.Split(val, "%")
	head := strings.Split(strings.TrimPrefix(parts[0], "#"), "#") // ["109", "0"] = ícone, tipo
	icon, _ := strconv.Atoi(head[0])
	field := func(i int) string {
		if i < len(parts) {
			return strings.TrimSpace(parts[i])
		}
		return ""
	}
	var typ string
	switch icon {
	case 109:
		typ = TypeSSH
	case 140:
		typ = TypeSFTP
	case 98:
		typ = TypeTelnet
	default:
		return s, false, true
	}
	host := field(1)
	if host == "" {
		return s, false, true
	}
	port, _ := strconv.Atoi(field(2))
	s = Session{
		Type: typ, Name: name, Folder: folder, Host: host, Port: port,
		AuthMethod: "password",
	}
	if typ != TypeTelnet {
		s.User = field(3)
		if s.User == "" || s.User == "<default>" {
			s.User = "root" // o MobaXterm pergunta o usuário; o NxATerm precisa de um valor, é só editar depois
			s.Notes = "Usuário não informado no MobaXterm: ajuste antes de conectar."
		}
	}
	if typ == TypeSSH {
		s.StartupCommand = field(7)
		if key := field(14); key != "" {
			if strings.HasSuffix(strings.ToLower(key), ".ppk") || strings.Contains(key, "_") && strings.HasPrefix(key, "_") {
				note := "Chave original do MobaXterm: " + key + " (formato .ppk ou variável do MobaXterm; converta para OpenSSH e configure aqui)."
				if s.Notes != "" {
					note = s.Notes + "\n" + note
				}
				s.Notes = note
			} else {
				s.AuthMethod, s.KeyPath = "key", key
			}
		}
	}
	return s, true, false
}
