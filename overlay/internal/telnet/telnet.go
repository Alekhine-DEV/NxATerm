// Package telnet implementa o mínimo do protocolo Telnet (RFC 854/855) que um
// emulador de terminal precisa: remove as sequências de comando do fluxo,
// responde à negociação de opções e informa o tipo e o tamanho do terminal.
package telnet

import (
	"net"
	"sync"
)

// Bytes de comando do protocolo.
const (
	iac  = 255
	dont = 254
	do   = 253
	wont = 252
	will = 251
	sb   = 250
	se   = 240
)

// Opções que o NxATerm entende.
const (
	optEcho  = 1
	optSGA   = 3  // suppress go-ahead
	optTType = 24 // terminal type
	optNAWS  = 31 // tamanho da janela
)

const (
	stNormal = iota
	stIAC
	stOpt
	stSB
	stSBIAC
)

// Filter é a máquina de estados que separa dados de comandos Telnet.
// É seguro usar de várias goroutines.
type Filter struct {
	mu       sync.Mutex
	state    int
	cmd      byte
	sub      []byte
	lastCR   bool
	him      map[byte]bool // opções que o servidor ativou (WILL aceito)
	us       map[byte]bool // opções que nós ativamos (DO aceito)
	termType string
	cols     int
	rows     int
}

// NewFilter cria o filtro. termType é o valor enviado quando o servidor pergunta.
func NewFilter(termType string, cols, rows int) *Filter {
	return &Filter{termType: termType, cols: cols, rows: rows, him: map[byte]bool{}, us: map[byte]bool{}}
}

// Process recebe bytes do servidor e devolve o que deve ir para o terminal (data)
// e o que deve ser enviado de volta ao servidor (reply). Sequências cortadas
// entre duas chamadas são tratadas corretamente.
func (f *Filter) Process(in []byte) (data, reply []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, b := range in {
		switch f.state {
		case stNormal:
			switch {
			case b == iac:
				f.state = stIAC
			case f.lastCR && b == 0:
				// CR NUL = um CR "puro" (RFC 854); o NUL não é para exibir
				f.lastCR = false
			default:
				data = append(data, b)
				f.lastCR = b == '\r'
			}
		case stIAC:
			switch b {
			case iac:
				data = append(data, iac)
				f.lastCR = false
				f.state = stNormal
			case will, wont, do, dont:
				f.cmd = b
				f.state = stOpt
			case sb:
				f.sub = f.sub[:0]
				f.state = stSB
			default: // NOP, GA, etc.
				f.state = stNormal
			}
		case stOpt:
			reply = f.negotiate(reply, f.cmd, b)
			f.state = stNormal
		case stSB:
			if b == iac {
				f.state = stSBIAC
			} else {
				f.sub = append(f.sub, b)
			}
		case stSBIAC:
			switch b {
			case se:
				reply = f.subnegotiate(reply)
				f.state = stNormal
			case iac:
				f.sub = append(f.sub, iac)
				f.state = stSB
			default:
				f.state = stNormal
			}
		}
	}
	return data, reply
}

func (f *Filter) negotiate(reply []byte, cmd, opt byte) []byte {
	switch cmd {
	case will:
		if opt == optEcho || opt == optSGA {
			if !f.him[opt] {
				f.him[opt] = true
				return append(reply, iac, do, opt)
			}
			return reply
		}
		return append(reply, iac, dont, opt)
	case wont:
		if f.him[opt] {
			f.him[opt] = false
			return append(reply, iac, dont, opt)
		}
	case do:
		if opt == optTType || opt == optNAWS || opt == optSGA {
			if !f.us[opt] {
				f.us[opt] = true
				reply = append(reply, iac, will, opt)
				if opt == optNAWS {
					reply = append(reply, f.nawsLocked()...)
				}
			}
			return reply
		}
		return append(reply, iac, wont, opt)
	case dont:
		if f.us[opt] {
			f.us[opt] = false
			return append(reply, iac, wont, opt)
		}
	}
	return reply
}

func (f *Filter) subnegotiate(reply []byte) []byte {
	// IAC SB TERMINAL-TYPE SEND IAC SE  ->  IAC SB TERMINAL-TYPE IS <tipo> IAC SE
	if len(f.sub) >= 2 && f.sub[0] == optTType && f.sub[1] == 1 && f.us[optTType] {
		reply = append(reply, iac, sb, optTType, 0)
		reply = append(reply, f.termType...)
		reply = append(reply, iac, se)
	}
	return reply
}

func (f *Filter) nawsLocked() []byte {
	out := []byte{iac, sb, optNAWS}
	for _, v := range []int{f.cols, f.rows} {
		hi, lo := byte(v>>8), byte(v)
		for _, x := range []byte{hi, lo} {
			if x == iac {
				out = append(out, iac)
			}
			out = append(out, x)
		}
	}
	return append(out, iac, se)
}

// Resize guarda o novo tamanho e devolve a mensagem NAWS a enviar ao servidor
// (nil se o servidor não pediu esse recurso).
func (f *Filter) Resize(cols, rows int) []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cols, f.rows = cols, rows
	if !f.us[optNAWS] {
		return nil
	}
	return f.nawsLocked()
}

// Encode prepara o que o usuário digitou para ir ao servidor: duplica o 0xFF e
// transforma o CR (tecla Enter) em CR NUL, como o cliente telnet clássico.
func Encode(data []byte) []byte {
	out := make([]byte, 0, len(data)+4)
	for i := 0; i < len(data); i++ {
		b := data[i]
		switch {
		case b == iac:
			out = append(out, iac, iac)
		case b == '\r':
			out = append(out, '\r')
			if i+1 < len(data) && data[i+1] == '\n' {
				continue // CR LF já é uma quebra de linha completa
			}
			out = append(out, 0)
		default:
			out = append(out, b)
		}
	}
	return out
}

// Conn envolve uma conexão TCP aplicando o Filter na leitura e o Encode na escrita.
type Conn struct {
	c       net.Conn
	f       *Filter
	wmu     sync.Mutex
	raw     []byte
	pending []byte
	rerr    error
}

// NewConn cria a conexão Telnet sobre c.
func NewConn(c net.Conn, termType string, cols, rows int) *Conn {
	return &Conn{c: c, f: NewFilter(termType, cols, rows), raw: make([]byte, 4096)}
}

// Read devolve só os dados do usuário; a negociação é tratada internamente.
func (t *Conn) Read(p []byte) (int, error) {
	for {
		if len(t.pending) > 0 {
			n := copy(p, t.pending)
			t.pending = t.pending[n:]
			return n, nil
		}
		if t.rerr != nil {
			return 0, t.rerr
		}
		n, err := t.c.Read(t.raw)
		if n > 0 {
			data, reply := t.f.Process(t.raw[:n])
			if len(reply) > 0 {
				t.writeRaw(reply)
			}
			t.pending = data
		}
		if err != nil {
			t.rerr = err
		}
	}
}

// Write envia texto digitado (com escape de IAC e CR).
func (t *Conn) Write(p []byte) (int, error) {
	if _, err := t.writeRaw(Encode(p)); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (t *Conn) writeRaw(b []byte) (int, error) {
	t.wmu.Lock()
	defer t.wmu.Unlock()
	return t.c.Write(b)
}

// Resize avisa o servidor da nova dimensão do terminal.
func (t *Conn) Resize(cols, rows int) error {
	if msg := t.f.Resize(cols, rows); msg != nil {
		_, err := t.writeRaw(msg)
		return err
	}
	return nil
}

// Close fecha o socket.
func (t *Conn) Close() error { return t.c.Close() }
