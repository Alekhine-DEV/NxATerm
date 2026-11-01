// Package telnet implementa o mínimo do protocolo Telnet (RFC 854/855) que um
// emulador de terminal precisa: remove as sequências de comando do fluxo,
// responde à negociação de opções e informa o tipo e o tamanho do terminal.
package telnet

import (
	"net"
	"sync"
	"time"
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
	optEcho   = 1
	optBinary = 0  // transmissão binária (sem CR NUL)
	optSGA    = 3  // suppress go-ahead
	optTType  = 24 // terminal type
	optNAWS   = 31 // tamanho da janela
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

	wantBinary bool // pedimos o modo binário (Zmodem)
	askedDo    bool // já enviamos DO BINARY
	askedWill  bool // já enviamos WILL BINARY
}

// binaryOut diz se estamos enviando em binário (sem CR NUL).
func (f *Filter) binaryOut() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.us[optBinary]
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
			case f.lastCR && b == 0 && !f.him[optBinary]:
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
	if opt == optBinary {
		return f.negotiateBinary(reply, cmd)
	}
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

// negotiateBinary trata a opção BINARY (RFC 856). Só aceitamos quando pedimos (SetBinary).
func (f *Filter) negotiateBinary(reply []byte, cmd byte) []byte {
	switch cmd {
	case will: // o servidor quer enviar em binário
		if f.wantBinary {
			f.him[optBinary] = true
			if !f.askedDo {
				f.askedDo = true
				return append(reply, iac, do, optBinary)
			}
			return reply
		}
		return append(reply, iac, dont, optBinary)
	case wont:
		if f.him[optBinary] {
			f.him[optBinary] = false
			f.askedDo = false
			return append(reply, iac, dont, optBinary)
		}
	case do: // o servidor aceita receber em binário
		if f.wantBinary {
			f.us[optBinary] = true
			if !f.askedWill {
				f.askedWill = true
				return append(reply, iac, will, optBinary)
			}
			return reply
		}
		return append(reply, iac, wont, optBinary)
	case dont:
		if f.us[optBinary] {
			f.us[optBinary] = false
			f.askedWill = false
			return append(reply, iac, wont, optBinary)
		}
	}
	return reply
}

// SetBinary liga ou desliga o pedido de modo binário e devolve o que enviar ao servidor.
func (f *Filter) SetBinary(on bool) []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.wantBinary = on
	var msg []byte
	if on {
		if !f.askedDo {
			f.askedDo = true
			msg = append(msg, iac, do, optBinary)
		}
		if !f.askedWill {
			f.askedWill = true
			msg = append(msg, iac, will, optBinary)
		}
		return msg
	}
	if f.askedDo {
		f.askedDo, f.him[optBinary] = false, false
		msg = append(msg, iac, dont, optBinary)
	}
	if f.askedWill {
		f.askedWill, f.us[optBinary] = false, false
		msg = append(msg, iac, wont, optBinary)
	}
	return msg
}

// BinaryReady diz se o servidor confirmou o modo binário nos dois sentidos.
func (f *Filter) BinaryReady() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.him[optBinary] && f.us[optBinary]
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
func Encode(data []byte) []byte { return encode(data, false) }

// EncodeBinary só duplica o 0xFF: os bytes seguem como estão (modo binário, usado pelo Zmodem).
func EncodeBinary(data []byte) []byte { return encode(data, true) }

func encode(data []byte, binary bool) []byte {
	out := make([]byte, 0, len(data)+4)
	for i := 0; i < len(data); i++ {
		b := data[i]
		switch {
		case b == iac:
			out = append(out, iac, iac)
		case b == '\r' && !binary:
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

// Write envia texto digitado (com escape de IAC e CR). Em modo binário só o IAC é duplicado.
func (t *Conn) Write(p []byte) (int, error) {
	enc := Encode
	if t.f.binaryOut() {
		enc = EncodeBinary
	}
	if _, err := t.writeRaw(enc(p)); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (t *Conn) writeRaw(b []byte) (int, error) {
	t.wmu.Lock()
	defer t.wmu.Unlock()
	return t.c.Write(b)
}

// WriteBinary envia bytes sem converter CR (só duplica o 0xFF). É o caminho do Zmodem.
func (t *Conn) WriteBinary(p []byte) (int, error) {
	if _, err := t.writeRaw(EncodeBinary(p)); err != nil {
		return 0, err
	}
	return len(p), nil
}

// SetBinary liga ou desliga o modo binário (RFC 856) e espera até 2 s a confirmação do servidor.
// Sem confirmação, segue assim mesmo: servidores simples já tratam os dados sem alteração.
func (t *Conn) SetBinary(on bool) error {
	if msg := t.f.SetBinary(on); len(msg) > 0 {
		if _, err := t.writeRaw(msg); err != nil {
			return err
		}
	}
	if !on {
		return nil
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if t.f.BinaryReady() {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return nil
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
