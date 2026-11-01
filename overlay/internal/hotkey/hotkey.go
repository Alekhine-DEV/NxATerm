// Package hotkey interpreta atalhos como "Ctrl+Alt+T" e os traduz para o Windows (RegisterHotKey),
// o X11 (XGrabKey) e o macOS (RegisterEventHotKey).
package hotkey

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Modificadores do RegisterHotKey.
const (
	ModAlt      = 0x0001
	ModControl  = 0x0002
	ModShift    = 0x0004
	ModWin      = 0x0008
	ModNoRepeat = 0x4000
)

var named = map[string]uint32{
	"SPACE": 0x20, "ESPACO": 0x20, "ENTER": 0x0D, "TAB": 0x09, "ESC": 0x1B, "ESCAPE": 0x1B,
	"BACKSPACE": 0x08, "INSERT": 0x2D, "DELETE": 0x2E, "HOME": 0x24, "END": 0x23,
	"PAGEUP": 0x21, "PAGEDOWN": 0x22, "UP": 0x26, "DOWN": 0x28, "LEFT": 0x25, "RIGHT": 0x27,
	"`": 0xC0, "'": 0xDE, ";": 0xBA, "=": 0xBB, ",": 0xBC, "-": 0xBD, ".": 0xBE, "/": 0xBF,
	"[": 0xDB, "\\": 0xDC, "]": 0xDD,
}

// Combo é um atalho já interpretado, sem depender do sistema.
type Combo struct {
	Ctrl, Alt, Shift, Super bool
	Key                     string // letra ou número em maiúsculas, F1..F24, nome (SPACE, ENTER...) ou símbolo
}

// ParseCombo lê um atalho como "Ctrl+Alt+T". Exige ao menos Ctrl, Alt ou Win/Cmd junto com a tecla.
func ParseCombo(spec string) (Combo, error) {
	var c Combo
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return c, errors.New("atalho vazio")
	}
	for _, part := range strings.Split(spec, "+") {
		p := strings.ToUpper(strings.TrimSpace(part))
		switch p {
		case "CTRL", "CONTROL", "CTL":
			c.Ctrl = true
		case "ALT", "OPTION", "OPT":
			c.Alt = true
		case "SHIFT":
			c.Shift = true
		case "WIN", "SUPER", "META", "CMD", "COMMAND":
			c.Super = true
		case "":
			return c, fmt.Errorf("atalho inválido: %q", spec)
		default:
			if c.Key != "" {
				return c, fmt.Errorf("mais de uma tecla em %q", spec)
			}
			c.Key = p
		}
	}
	if c.Key == "" {
		return c, fmt.Errorf("falta a tecla em %q", spec)
	}
	if !c.Ctrl && !c.Alt && !c.Super {
		return c, errors.New("use Ctrl, Alt ou Win junto com a tecla")
	}
	if _, ok := c.windowsVK(); !ok {
		return c, fmt.Errorf("tecla desconhecida: %q", c.Key)
	}
	return c, nil
}

// windowsVK devolve o código virtual de tecla do Windows (também serve para validar o nome).
func (c Combo) windowsVK() (uint32, bool) {
	key := c.Key
	switch {
	case len(key) == 1 && key[0] >= 'A' && key[0] <= 'Z', len(key) == 1 && key[0] >= '0' && key[0] <= '9':
		return uint32(key[0]), true
	case len(key) > 1 && key[0] == 'F':
		n, e := strconv.Atoi(key[1:])
		if e != nil || n < 1 || n > 24 {
			return 0, false
		}
		return uint32(0x70 + n - 1), true
	}
	v, ok := named[key]
	return v, ok
}

// Parse devolve os modificadores e o código da tecla para o RegisterHotKey do Windows.
func Parse(spec string) (mods uint32, vk uint32, err error) {
	c, err := ParseCombo(spec)
	if err != nil {
		return 0, 0, err
	}
	if c.Ctrl {
		mods |= ModControl
	}
	if c.Alt {
		mods |= ModAlt
	}
	if c.Shift {
		mods |= ModShift
	}
	if c.Super {
		mods |= ModWin
	}
	vk, _ = c.windowsVK()
	return mods | ModNoRepeat, vk, nil
}

var x11Named = map[string]uint32{
	"SPACE": 0x20, "ESPACO": 0x20, "ENTER": 0xff0d, "TAB": 0xff09, "ESC": 0xff1b, "ESCAPE": 0xff1b,
	"BACKSPACE": 0xff08, "INSERT": 0xff63, "DELETE": 0xffff, "HOME": 0xff50, "END": 0xff57,
	"PAGEUP": 0xff55, "PAGEDOWN": 0xff56, "UP": 0xff52, "DOWN": 0xff54, "LEFT": 0xff51, "RIGHT": 0xff53,
	"`": 0x60, "'": 0x27, ";": 0x3b, "=": 0x3d, ",": 0x2c, "-": 0x2d, ".": 0x2e, "/": 0x2f,
	"[": 0x5b, "\\": 0x5c, "]": 0x5d,
}

// X11Keysym devolve o keysym da tecla (letras em minúsculas, como o X11 espera).
func (c Combo) X11Keysym() (uint32, bool) {
	key := c.Key
	switch {
	case len(key) == 1 && key[0] >= 'A' && key[0] <= 'Z':
		return uint32(key[0]) + 0x20, true
	case len(key) == 1 && key[0] >= '0' && key[0] <= '9':
		return uint32(key[0]), true
	case len(key) > 1 && key[0] == 'F':
		n, e := strconv.Atoi(key[1:])
		if e != nil || n < 1 || n > 24 {
			return 0, false
		}
		return uint32(0xffbe + n - 1), true
	}
	v, ok := x11Named[key]
	return v, ok
}

// Códigos de tecla virtuais do macOS (teclado ANSI, os mesmos do Carbon/HIToolbox).
var macKeys = map[string]uint32{
	"A": 0, "S": 1, "D": 2, "F": 3, "H": 4, "G": 5, "Z": 6, "X": 7, "C": 8, "V": 9, "B": 11, "Q": 12,
	"W": 13, "E": 14, "R": 15, "Y": 16, "T": 17, "1": 18, "2": 19, "3": 20, "4": 21, "6": 22, "5": 23,
	"=": 24, "9": 25, "7": 26, "-": 27, "8": 28, "0": 29, "]": 30, "O": 31, "U": 32, "[": 33, "I": 34,
	"P": 35, "ENTER": 36, "L": 37, "J": 38, "'": 39, "K": 40, ";": 41, "\\": 42, ",": 43, "/": 44,
	"N": 45, "M": 46, ".": 47, "TAB": 48, "SPACE": 49, "ESPACO": 49, "`": 50, "BACKSPACE": 51, "ESC": 53, "ESCAPE": 53,
	"F1": 122, "F2": 120, "F3": 99, "F4": 118, "F5": 96, "F6": 97, "F7": 98, "F8": 100, "F9": 101,
	"F10": 109, "F11": 103, "F12": 111, "HOME": 115, "PAGEUP": 116, "DELETE": 117, "END": 119,
	"PAGEDOWN": 121, "LEFT": 123, "RIGHT": 124, "DOWN": 125, "UP": 126,
}

// MacKeycode devolve o código de tecla virtual do macOS.
func (c Combo) MacKeycode() (uint32, bool) {
	v, ok := macKeys[c.Key]
	return v, ok
}
