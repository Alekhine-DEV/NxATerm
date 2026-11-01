package store

import (
	"path/filepath"
	"strings"
	"testing"
)

func newTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "sub", "sessions.json")
	s, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	return s, p
}

func TestSaveDefaultsAndPersistence(t *testing.T) {
	s, p := newTestStore(t)
	got, err := s.Save(Session{Host: " srv1 ", User: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID == "" || got.Port != 22 || got.AuthMethod != "password" || got.Name != "root@srv1" {
		t.Fatalf("defaults erradas: %+v", got)
	}
	s2, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if l := s2.List(); len(l) != 1 || l[0].ID != got.ID {
		t.Fatalf("não persistiu: %+v", l)
	}
}

func TestNoSessionLimit(t *testing.T) {
	s, p := newTestStore(t)
	for i := 0; i < 500; i++ {
		_, err := s.Save(Session{Host: "h", User: "u", Port: 1 + i%65535, Name: "s"})
		if err != nil {
			t.Fatal(err)
		}
	}
	s2, _ := Open(p)
	if n := len(s2.List()); n != 500 {
		t.Fatalf("esperava 500 sessões, veio %d", n)
	}
}

func TestValidation(t *testing.T) {
	s, _ := newTestStore(t)
	cases := []Session{
		{User: "u"},
		{Host: "h"},
		{Host: "h", User: "u", Port: 70000},
		{Host: "h", User: "u", AuthMethod: "key"},
		{Host: "h", User: "u", AuthMethod: "magia"},
	}
	for i, c := range cases {
		if _, err := s.Save(c); err == nil {
			t.Errorf("caso %d deveria falhar: %+v", i, c)
		}
	}
}

func TestUpdateAndDelete(t *testing.T) {
	s, _ := newTestStore(t)
	a, _ := s.Save(Session{Host: "a", User: "u"})
	a.Name = "Renomeada"
	if _, err := s.Save(a); err != nil {
		t.Fatal(err)
	}
	if l := s.List(); len(l) != 1 || l[0].Name != "Renomeada" {
		t.Fatalf("update falhou: %+v", l)
	}
	if _, err := s.Save(Session{ID: "naoexiste", Host: "a", User: "u"}); err == nil {
		t.Fatal("update de ID inexistente deveria falhar")
	}
	if err := s.Delete(a.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(a.ID); err == nil {
		t.Fatal("delete repetido deveria falhar")
	}
}

func TestListOrder(t *testing.T) {
	s, _ := newTestStore(t)
	s.Save(Session{Name: "b", Folder: "Z", Host: "h", User: "u"})
	s.Save(Session{Name: "a", Folder: "Z", Host: "h", User: "u", Port: 2})
	s.Save(Session{Name: "c", Folder: "A", Host: "h", User: "u", Port: 3})
	l := s.List()
	got := l[0].Name + l[1].Name + l[2].Name
	if got != "cab" {
		t.Fatalf("ordem errada: %s", got)
	}
}

func TestImportSkipsDuplicates(t *testing.T) {
	s, _ := newTestStore(t)
	s.Save(Session{Host: "dup", User: "u"})
	n, err := s.Import([]Session{
		{Host: "dup", User: "u"},
		{Host: "novo", User: "u"},
		{Host: "", User: "u"}, // inválida, ignorada
	})
	if err != nil || n != 1 {
		t.Fatalf("import: n=%d err=%v", n, err)
	}
	if len(s.List()) != 2 {
		t.Fatalf("esperava 2 sessões")
	}
}

func TestParseSSHConfig(t *testing.T) {
	cfg := `
# comentário
Host *
    ServerAliveInterval 30

Host web db
    HostName 10.0.0.5
    User deploy
    Port 2222
    IdentityFile ~/.ssh/id_ed25519

Host=sem-user
    HostName 1.2.3.4

Host "curinga-*" bastion
    HostName=bastion.example.com
    User ops

Match host foo
    User ignorado
`
	got := ParseSSHConfig(strings.NewReader(cfg), "/home/x")
	byName := map[string]Session{}
	for _, s := range got {
		byName[s.Name] = s
	}
	if len(got) != 3 {
		t.Fatalf("esperava 3 sessões (web, db, bastion), veio %d: %+v", len(got), got)
	}
	web := byName["web"]
	if web.Host != "10.0.0.5" || web.User != "deploy" || web.Port != 2222 ||
		web.AuthMethod != "key" || web.KeyPath != filepath.Join("/home/x", ".ssh/id_ed25519") {
		t.Fatalf("web errado: %+v", web)
	}
	if byName["db"].Host != "10.0.0.5" {
		t.Fatalf("db errado: %+v", byName["db"])
	}
	b := byName["bastion"]
	if b.Host != "bastion.example.com" || b.User != "ops" || b.AuthMethod != "password" {
		t.Fatalf("bastion errado: %+v", b)
	}
}

func TestSessionTypes(t *testing.T) {
	s, _ := newTestStore(t)

	old, err := s.Save(Session{Host: "h", User: "u"})
	if err != nil || old.Type != TypeSSH {
		t.Fatalf("perfil sem tipo deve virar ssh: %+v %v", old, err)
	}

	tel, err := s.Save(Session{Type: "telnet", Host: "10.0.0.1"})
	if err != nil || tel.Port != 23 || tel.Name != "10.0.0.1" || tel.User != "" {
		t.Fatalf("telnet: %+v %v", tel, err)
	}
	if _, err := s.Save(Session{Type: "telnet"}); err == nil {
		t.Error("telnet sem host deveria falhar")
	}

	sf, err := s.Save(Session{Type: "SFTP", Host: "h", User: "u"})
	if err != nil || sf.Type != TypeSFTP || sf.Port != 22 || sf.AuthMethod != "password" {
		t.Fatalf("sftp: %+v %v", sf, err)
	}
	if _, err := s.Save(Session{Type: "sftp", Host: "h"}); err == nil {
		t.Error("sftp sem usuário deveria falhar")
	}

	ser, err := s.Save(Session{Type: "serial", SerialPort: "COM3"})
	if err != nil || ser.BaudRate != 9600 || ser.DataBits != 8 || ser.Parity != "none" || ser.StopBits != "1" || ser.Name != "COM3 (9600)" {
		t.Fatalf("serial: %+v %v", ser, err)
	}
	for i, bad := range []Session{
		{Type: "serial"},
		{Type: "serial", SerialPort: "COM1", BaudRate: 10},
		{Type: "serial", SerialPort: "COM1", DataBits: 9},
		{Type: "serial", SerialPort: "COM1", Parity: "x"},
		{Type: "serial", SerialPort: "COM1", StopBits: "3"},
	} {
		if _, err := s.Save(bad); err == nil {
			t.Errorf("serial inválido %d deveria falhar: %+v", i, bad)
		}
	}

	sh, err := s.Save(Session{Type: "shell", Shell: " cmd.exe "})
	if err != nil || sh.Shell != "cmd.exe" || sh.Name != "Shell local" {
		t.Fatalf("shell: %+v %v", sh, err)
	}
	w, err := s.Save(Session{Type: "wsl", Distro: "Ubuntu"})
	if err != nil || w.Name != "WSL: Ubuntu" {
		t.Fatalf("wsl: %+v %v", w, err)
	}
	w2, _ := s.Save(Session{Type: "wsl"})
	if w2.Name != "WSL" {
		t.Fatalf("wsl padrão: %+v", w2)
	}

	if _, err := s.Save(Session{Type: "rdp", Host: "h", User: "u"}); err == nil {
		t.Error("tipo desconhecido deveria falhar")
	}
}

func TestFolders(t *testing.T) {
	s, p := newTestStore(t)
	if err := s.CreateFolder(`  A \ B/ `); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateFolder(""); err == nil {
		t.Error("pasta sem nome deveria falhar")
	}
	if f := s.Folders(); len(f) != 1 || f[0] != "A/B" {
		t.Fatalf("pastas: %v", f)
	}
	s.Save(Session{Host: "h1", User: "u", Folder: "A/B"})
	s.Save(Session{Host: "h2", User: "u", Folder: "A/B/C"})
	s.Save(Session{Host: "h3", User: "u", Folder: "Outra"})

	if err := s.RenameFolder("A/B", "A/B"); err != nil {
		t.Fatal(err)
	}
	if err := s.RenameFolder("A", "A/X"); err == nil {
		t.Error("mover para dentro de si mesma deveria falhar")
	}
	if err := s.RenameFolder("A", "Z"); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, x := range s.List() {
		got[x.Host+"="+x.Folder] = true
	}
	for _, w := range []string{"h1=Z/B", "h2=Z/B/C", "h3=Outra"} {
		if !got[w] {
			t.Errorf("faltou %s em %v", w, got)
		}
	}
	if f := s.Folders(); len(f) != 2 || f[0] != "Z" || f[1] != "Z/B" {
		t.Fatalf("pastas após renomear: %v", f)
	}

	// persistência
	s2, _ := Open(p)
	if f := s2.Folders(); len(f) != 2 {
		t.Fatalf("pastas não persistiram: %v", f)
	}

	// apagar mantendo as sessões: sobem um nível
	n, err := s.DeleteFolder("Z/B", false)
	if err != nil || n != 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	got = map[string]bool{}
	for _, x := range s.List() {
		got[x.Host+"="+x.Folder] = true
	}
	for _, w := range []string{"h1=Z", "h2=Z/C", "h3=Outra"} {
		if !got[w] {
			t.Errorf("após apagar, faltou %s em %v", w, got)
		}
	}
	if f := s.Folders(); len(f) != 1 || f[0] != "Z" {
		t.Fatalf("pastas após apagar: %v", f)
	}

	// apagar com as sessões
	n, err = s.DeleteFolder("Z", true)
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if l := s.List(); len(l) != 1 || l[0].Host != "h3" {
		t.Fatalf("sobraram: %+v", l)
	}
	if len(s.Folders()) != 0 {
		t.Fatalf("pastas: %v", s.Folders())
	}
}

func TestDeleteTopLevelFolderMovesToRoot(t *testing.T) {
	s, _ := newTestStore(t)
	s.Save(Session{Host: "h", User: "u", Folder: "Raiz/Sub"})
	if _, err := s.DeleteFolder("Raiz", false); err != nil {
		t.Fatal(err)
	}
	if l := s.List(); l[0].Folder != "Sub" {
		t.Fatalf("folder=%q", l[0].Folder)
	}
}

func TestExportImportRoundTrip(t *testing.T) {
	a, _ := newTestStore(t)
	a.CreateFolder("Vazia")
	a.Save(Session{Host: "h", User: "u", Folder: "P/Q", Name: "web"})
	a.Save(Session{Type: "serial", SerialPort: "COM3", BaudRate: 115200})
	a.Save(Session{Type: "wsl", Distro: "Ubuntu"})
	data, err := a.Export()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := newTestStore(t)
	added, skipped, err := b.ImportJSON(data)
	if err != nil || added != 3 || skipped != 0 {
		t.Fatalf("added=%d skipped=%d err=%v", added, skipped, err)
	}
	if f := b.Folders(); len(f) != 1 || f[0] != "Vazia" {
		t.Fatalf("pastas: %v", f)
	}
	added, skipped, _ = b.ImportJSON(data)
	if added != 0 || skipped != 3 {
		t.Fatalf("segunda importação: added=%d skipped=%d", added, skipped)
	}
	if _, _, err := b.ImportJSON([]byte("não é json")); err == nil {
		t.Error("deveria falhar")
	}
	// lista simples também vale
	c, _ := newTestStore(t)
	added, _, err = c.ImportJSON([]byte(`[{"host":"x","user":"y"}]`))
	if err != nil || added != 1 {
		t.Fatalf("lista simples: %d %v", added, err)
	}
}

const mobaSample = "[Bookmarks]\r\nSubRep=\r\nImgNum=42\r\nSolta=#109#0%10.0.0.1%22%root%%-1%-1%%%%%0%0%0%%%-1%-1%0%0%%1080%%0%0%1%#MobaFont%10%0%0%-1%15#0# #-1\r\n\r\n" +
	"[Bookmarks_1]\r\nSubRep=MAGENTO\\PROD\r\nImgNum=41\r\nLOJA 1 - PROD=#109#0%loja1.example.com%2222%deploy%%-1%-1%uptime%%%%0%0%0%C:\\Users\\x\\.ssh\\id_rsa%%-1%-1%0%0%%1080%%0%0%1%#MobaFont%10%0%0%-1%15#0# #-1\r\n" +
	"Chave PPK=#109#0%ppk.example.com%22%ana%%-1%-1%%%%%0%0%0%C:\\keys\\a.ppk%%-1%-1%0#0# #-1\r\n" +
	"Sem usuario=#109#0%semuser.example.com%22%%%-1%-1%%%%%0%0%0%%#0# #-1\r\n" +
	"Rede=#98#1%switch1%23%%#0# #-1\r\n" +
	"Arquivos=#140#7%files.example.com%22%bob%%#0# #-1\r\n" +
	"Remota=#91#4%rdp.example.com%3389%%#0# #-1\r\n" +
	"[Misc]\r\nSlashDir=x\r\n"

func TestParseMobaXterm(t *testing.T) {
	res, err := ParseMobaXterm(strings.NewReader(mobaSample))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Sessions) != 6 || res.Skipped != 1 {
		t.Fatalf("sessões=%d skipped=%d: %+v", len(res.Sessions), res.Skipped, res.Sessions)
	}
	by := map[string]Session{}
	for _, s := range res.Sessions {
		by[s.Name] = s
	}
	solta := by["Solta"]
	if solta.Host != "10.0.0.1" || solta.Port != 22 || solta.User != "root" || solta.Folder != "" || solta.Type != TypeSSH {
		t.Errorf("Solta: %+v", solta)
	}
	l1 := by["LOJA 1 - PROD"]
	if l1.Folder != "MAGENTO/PROD" || l1.Port != 2222 || l1.User != "deploy" || l1.StartupCommand != "uptime" ||
		l1.AuthMethod != "key" || l1.KeyPath != `C:\Users\x\.ssh\id_rsa` {
		t.Errorf("LOJA 1: %+v", l1)
	}
	if p := by["Chave PPK"]; p.AuthMethod != "password" || !strings.Contains(p.Notes, "a.ppk") {
		t.Errorf("PPK: %+v", p)
	}
	if u := by["Sem usuario"]; u.User == "" || u.Notes == "" {
		t.Errorf("Sem usuario: %+v", u)
	}
	if n := by["Rede"]; n.Type != TypeTelnet || n.Host != "switch1" || n.Port != 23 {
		t.Errorf("Rede: %+v", n)
	}
	if f := by["Arquivos"]; f.Type != TypeSFTP || f.User != "bob" {
		t.Errorf("Arquivos: %+v", f)
	}
	if len(res.Folders) != 1 || res.Folders[0] != "MAGENTO/PROD" {
		t.Errorf("pastas: %v", res.Folders)
	}
	// todas devem passar na validação do cadastro
	st, _ := newTestStore(t)
	added, _, err := st.ImportAll(res.Sessions, res.Folders)
	if err != nil || added != 6 {
		t.Fatalf("added=%d err=%v", added, err)
	}
}

func TestMobaLatin1(t *testing.T) {
	// "Produção" em Windows-1252: ç=0xE7, ã=0xE3
	raw := []byte("[Bookmarks_1]\r\nSubRep=Produ\xe7\xe3o\r\nsrv=#109#0%h%22%u%\r\n")
	res, _ := ParseMobaXterm(strings.NewReader(string(raw)))
	if len(res.Sessions) != 1 || res.Sessions[0].Folder != "Produção" {
		t.Fatalf("%+v", res.Sessions)
	}
}

func TestParsePuTTYReg(t *testing.T) {
	out := "\r\nHKEY_CURRENT_USER\\Software\\SimonTatham\\PuTTY\\Sessions\r\n\r\n" +
		"HKEY_CURRENT_USER\\Software\\SimonTatham\\PuTTY\\Sessions\\Meu%20Servidor\r\n" +
		"    HostName    REG_SZ    192.168.0.9\r\n" +
		"    Protocol    REG_SZ    ssh\r\n" +
		"    PortNumber    REG_DWORD    0x16\r\n" +
		"    UserName    REG_SZ    admin\r\n" +
		"    PublicKeyFile    REG_SZ    C:\\keys\\id.ppk\r\n\r\n" +
		"HKEY_CURRENT_USER\\Software\\SimonTatham\\PuTTY\\Sessions\\Switch\r\n" +
		"    HostName    REG_SZ    10.1.1.1\r\n" +
		"    Protocol    REG_SZ    telnet\r\n" +
		"    PortNumber    REG_DWORD    0x17\r\n\r\n" +
		"HKEY_CURRENT_USER\\Software\\SimonTatham\\PuTTY\\Sessions\\Serial1\r\n" +
		"    HostName    REG_SZ    COM1\r\n" +
		"    Protocol    REG_SZ    serial\r\n"
	got := ParsePuTTYReg(out)
	if len(got) != 2 {
		t.Fatalf("%+v", got)
	}
	a := got[0]
	if a.Name != "Meu Servidor" || a.Host != "192.168.0.9" || a.Port != 22 || a.User != "admin" || a.Folder != "PuTTY" || !strings.Contains(a.Notes, "id.ppk") {
		t.Errorf("a=%+v", a)
	}
	if b := got[1]; b.Type != TypeTelnet || b.Port != 23 || b.Name != "Switch" {
		t.Errorf("b=%+v", b)
	}
}

func TestPuTTYSession(t *testing.T) {
	s, ok := PuTTYSession("prod", map[string]string{"HostName": "10.0.0.5", "PortNumber": "16", "UserName": "deploy", "PublicKeyFile": `C:\k\id_ed25519`})
	if !ok || s.Type != TypeSSH || s.Port != 22 || s.User != "deploy" || s.AuthMethod != "key" || s.Folder != "PuTTY" {
		t.Fatalf("ssh: %+v ok=%v", s, ok)
	}
	if s, ok := PuTTYSession("sw", map[string]string{"HostName": "sw1", "Protocol": "telnet", "PortNumber": "17"}); !ok || s.Type != TypeTelnet || s.Port != 23 {
		t.Fatalf("telnet: %+v ok=%v", s, ok)
	}
	for _, v := range []map[string]string{{"HostName": ""}, {"HostName": "h", "Protocol": "serial"}, {"HostName": "h", "Protocol": "raw"}} {
		if _, ok := PuTTYSession("x", v); ok {
			t.Fatalf("deveria ignorar %v", v)
		}
	}
}

func TestNormalizeConnectionOptions(t *testing.T) {
	s, err := Normalize(Session{Type: "ssh", Host: "h", User: "u", AuthMethod: "agent", Encoding: "UTF-8", TermType: "vt100",
		Env: " LANG=pt_BR.UTF-8 \r\n\n FOO=bar=baz ", X11: true, Proxy: " socks5://p:1080 "})
	if err != nil {
		t.Fatal(err)
	}
	if s.AuthMethod != "agent" || s.Encoding != "" || s.TermType != "vt100" || !s.X11 || s.Proxy != "socks5://p:1080" {
		t.Fatalf("%+v", s)
	}
	if s.Env != "LANG=pt_BR.UTF-8\nFOO=bar=baz" {
		t.Fatalf("env = %q", s.Env)
	}
	pairs := EnvPairs(s.Env)
	if len(pairs) != 2 || pairs[1] != [2]string{"FOO", "bar=baz"} {
		t.Fatalf("%v", pairs)
	}
	if _, err := Normalize(Session{Type: "ssh", Host: "h", User: "u", Env: "1X=2"}); err == nil {
		t.Error("nome de variável inválido deveria falhar")
	}
	if _, err := Normalize(Session{Type: "ssh", Host: "h", User: "u", TermType: "x y"}); err == nil {
		t.Error("TERM inválido deveria falhar")
	}
	if _, err := Normalize(Session{ID: "a", Type: "ssh", Host: "h", User: "u", JumpHost: "a"}); err == nil {
		t.Error("salto para si mesmo deveria falhar")
	}
	tn, _ := Normalize(Session{Type: "telnet", Host: "h", X11: true, Proxy: "none"})
	if tn.X11 || tn.Proxy != "none" {
		t.Fatalf("%+v", tn)
	}
	sh, _ := Normalize(Session{Type: "shell", Proxy: "socks5://x:1", JumpHost: "z", Env: "A=1", StartDir: " /tmp "})
	if sh.Proxy != "" || sh.JumpHost != "" || sh.Env != "A=1" || sh.StartDir != "/tmp" {
		t.Fatalf("%+v", sh)
	}
}

func TestNormalizeFTP(t *testing.T) {
	s, err := Normalize(Session{Type: "ftp", Host: " ftp.exemplo.com "})
	if err != nil || s.Port != 21 || s.Name != "ftp.exemplo.com" || s.AuthMethod != "password" || s.User != "" {
		t.Fatalf("%+v %v", s, err)
	}
	s, err = Normalize(Session{Type: "ftp", Host: "h", User: "u", FTPSecurity: "implicit"})
	if err != nil || s.Port != 990 || s.Name != "u@h" {
		t.Fatalf("%+v %v", s, err)
	}
	if _, err := Normalize(Session{Type: "ftp", Host: "h", FTPSecurity: "ssl3"}); err == nil {
		t.Fatal("deveria recusar segurança inválida")
	}
	s, _ = Normalize(Session{Type: "ssh", Host: "h", User: "u", FTPSecurity: "explicit", FTPInsecure: true})
	if s.FTPSecurity != "" || s.FTPInsecure {
		t.Fatal("campos de FTP não valem para SSH")
	}
}

func TestColorAndTags(t *testing.T) {
	s, err := Normalize(Session{Host: "h", User: "u", Color: " #FF8800 ", Tags: []string{" #prod ", "Prod", "web,db", "", "a"}})
	if err != nil {
		t.Fatal(err)
	}
	if s.Color != "#ff8800" {
		t.Errorf("cor = %q", s.Color)
	}
	if len(s.Tags) != 3 || s.Tags[0] != "prod" || s.Tags[1] != "web db" || s.Tags[2] != "a" {
		t.Errorf("tags = %q", s.Tags)
	}
	s, _ = Normalize(Session{Host: "h", User: "u", Color: "vermelho"})
	if s.Color != "" {
		t.Errorf("cor inválida ficou: %q", s.Color)
	}
}
