package telnet

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"
)

func TestPlainDataPassesThrough(t *testing.T) {
	f := NewFilter("xterm", 80, 24)
	data, reply := f.Process([]byte("login: "))
	if string(data) != "login: " || len(reply) != 0 {
		t.Fatalf("data=%q reply=%v", data, reply)
	}
}

func TestEscapedIACAndCRNul(t *testing.T) {
	f := NewFilter("xterm", 80, 24)
	data, _ := f.Process([]byte{'a', iac, iac, 'b', '\r', 0, 'c', '\r', '\n'})
	want := []byte{'a', 0xFF, 'b', '\r', 'c', '\r', '\n'}
	if !bytes.Equal(data, want) {
		t.Fatalf("got %v want %v", data, want)
	}
}

func TestNegotiation(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want []byte
	}{
		{"server will echo -> do echo", []byte{iac, will, optEcho}, []byte{iac, do, optEcho}},
		{"server will sga -> do sga", []byte{iac, will, optSGA}, []byte{iac, do, optSGA}},
		{"server will unknown -> dont", []byte{iac, will, 99}, []byte{iac, dont, 99}},
		{"server do ttype -> will", []byte{iac, do, optTType}, []byte{iac, will, optTType}},
		{"server do echo -> wont", []byte{iac, do, optEcho}, []byte{iac, wont, optEcho}},
		{"server do unknown -> wont", []byte{iac, do, 77}, []byte{iac, wont, 77}},
		{"server do naws -> will + tamanho", []byte{iac, do, optNAWS},
			[]byte{iac, will, optNAWS, iac, sb, optNAWS, 0, 80, 0, 24, iac, se}},
	}
	for _, c := range cases {
		f := NewFilter("xterm", 80, 24)
		data, reply := f.Process(c.in)
		if len(data) != 0 {
			t.Errorf("%s: data inesperado %v", c.name, data)
		}
		if !bytes.Equal(reply, c.want) {
			t.Errorf("%s: reply %v, esperado %v", c.name, reply, c.want)
		}
	}
}

func TestNoNegotiationLoop(t *testing.T) {
	f := NewFilter("xterm", 80, 24)
	_, r1 := f.Process([]byte{iac, will, optEcho})
	_, r2 := f.Process([]byte{iac, will, optEcho})
	if len(r1) == 0 || len(r2) != 0 {
		t.Fatalf("o segundo WILL ECHO não pode gerar resposta: %v %v", r1, r2)
	}
	// WONT sem ter ativado antes é ignorado
	_, r3 := f.Process([]byte{iac, wont, 55})
	if len(r3) != 0 {
		t.Fatalf("WONT de opção não ativada gerou resposta: %v", r3)
	}
	// WONT de opção ativada é confirmado com DONT
	_, r4 := f.Process([]byte{iac, wont, optEcho})
	if !bytes.Equal(r4, []byte{iac, dont, optEcho}) {
		t.Fatalf("r4=%v", r4)
	}
}

func TestSplitSequenceAcrossReads(t *testing.T) {
	f := NewFilter("xterm", 80, 24)
	var reply []byte
	var data []byte
	for _, chunk := range [][]byte{{'h', iac}, {do}, {optTType, 'i'}} {
		d, r := f.Process(chunk)
		data = append(data, d...)
		reply = append(reply, r...)
	}
	if string(data) != "hi" || !bytes.Equal(reply, []byte{iac, will, optTType}) {
		t.Fatalf("data=%q reply=%v", data, reply)
	}
}

func TestTerminalTypeSubnegotiation(t *testing.T) {
	f := NewFilter("xterm-256color", 80, 24)
	f.Process([]byte{iac, do, optTType})
	_, reply := f.Process([]byte{iac, sb, optTType, 1, iac, se})
	want := append([]byte{iac, sb, optTType, 0}, []byte("xterm-256color")...)
	want = append(want, iac, se)
	if !bytes.Equal(reply, want) {
		t.Fatalf("reply=%v", reply)
	}
}

func TestTerminalTypeIgnoredWhenNotAgreed(t *testing.T) {
	f := NewFilter("xterm", 80, 24)
	_, reply := f.Process([]byte{iac, sb, optTType, 1, iac, se})
	if len(reply) != 0 {
		t.Fatalf("respondeu sem ter aceito a opção: %v", reply)
	}
}

func TestResizeNAWS(t *testing.T) {
	f := NewFilter("xterm", 80, 24)
	if msg := f.Resize(100, 30); msg != nil {
		t.Fatalf("sem a opção ativa não deve enviar nada: %v", msg)
	}
	f.Process([]byte{iac, do, optNAWS})
	msg := f.Resize(255, 300) // 255 precisa de escape; 300 = 0x012C
	want := []byte{iac, sb, optNAWS, 0, iac, iac, 1, 44, iac, se}
	if !bytes.Equal(msg, want) {
		t.Fatalf("msg=%v want=%v", msg, want)
	}
}

func TestEncode(t *testing.T) {
	cases := []struct{ in, want []byte }{
		{[]byte("ls\r"), []byte{'l', 's', '\r', 0}},
		{[]byte{0xFF}, []byte{0xFF, 0xFF}},
		{[]byte("a\r\nb"), []byte{'a', '\r', '\n', 'b'}},
		{[]byte("x"), []byte("x")},
	}
	for _, c := range cases {
		if got := Encode(c.in); !bytes.Equal(got, c.want) {
			t.Errorf("Encode(%v)=%v, esperado %v", c.in, got, c.want)
		}
	}
}

// Conversa completa por uma conexão em memória: o "servidor" negocia, manda um
// banner e depois recebe o que o usuário digita.
func TestConnEndToEnd(t *testing.T) {
	cli, srv := net.Pipe()
	defer cli.Close()
	defer srv.Close()

	got := make(chan []byte, 1)
	go func() {
		srv.Write([]byte{iac, do, optTType, iac, will, optEcho})
		reply := make([]byte, 6)
		io.ReadFull(srv, reply) // WILL TTYPE + DO ECHO
		srv.Write([]byte("Welcome\r\n"))
		buf := make([]byte, 4)
		io.ReadFull(srv, buf)
		got <- append(reply, buf...)
	}()

	c := NewConn(cli, "xterm", 80, 24)
	buf := make([]byte, 64)
	n, err := c.Read(buf)
	if err != nil || string(buf[:n]) != "Welcome\r\n" {
		t.Fatalf("n=%d err=%v data=%q", n, err, buf[:n])
	}
	if _, err := c.Write([]byte("ok\r")); err != nil {
		t.Fatal(err)
	}
	select {
	case b := <-got:
		want := []byte{iac, will, optTType, iac, do, optEcho, 'o', 'k', '\r', 0}
		if !bytes.Equal(b, want) {
			t.Fatalf("servidor recebeu %v, esperado %v", b, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("tempo esgotado")
	}
}

func TestConnSmallReadBuffer(t *testing.T) {
	cli, srv := net.Pipe()
	defer cli.Close()
	go func() { srv.Write([]byte("abcdef")); srv.Close() }()
	c := NewConn(cli, "xterm", 80, 24)
	var all []byte
	small := make([]byte, 2)
	for {
		n, err := c.Read(small)
		all = append(all, small[:n]...)
		if err != nil {
			break
		}
	}
	if string(all) != "abcdef" {
		t.Fatalf("all=%q", all)
	}
}
