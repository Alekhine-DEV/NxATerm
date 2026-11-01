// Package hotkey interpreta atalhos como "Ctrl+Alt+T" para o RegisterHotKey do Windows.
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
	"`": 0xC0, "'": 0xC0, ";": 0xBA, "=": 0xBB, ",": 0xBC, "-": 0xBD, ".": 0xBE, "/": 0xBF,
	"[": 0xDB, "\\": 0xDC, "]": 0xDD,
}

// Parse devolve os modificadores e o código da tecla. Exige ao menos um modificador.
func Parse(spec string) (mods uint32, vk uint32, err error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return 0, 0, errors.New("atalho vazio")
	}
	var key string
	for _, part := range strings.Split(spec, "+") {
		p := strings.ToUpper(strings.TrimSpace(part))
		switch p {
		case "CTRL", "CONTROL", "CTL":
			mods |= ModControl
		case "ALT":
			mods |= ModAlt
		case "SHIFT":
			mods |= ModShift
		case "WIN", "SUPER", "META", "CMD":
			mods |= ModWin
		case "":
			return 0, 0, fmt.Errorf("atalho inválido: %q", spec)
		default:
			if key != "" {
				return 0, 0, fmt.Errorf("mais de uma tecla em %q", spec)
			}
			key = p
		}
	}
	if key == "" {
		return 0, 0, fmt.Errorf("falta a tecla em %q", spec)
	}
	if mods&(ModControl|ModAlt|ModWin) == 0 {
		return 0, 0, errors.New("use Ctrl, Alt ou Win junto com a tecla")
	}
	switch {
	case len(key) == 1 && key[0] >= 'A' && key[0] <= 'Z', len(key) == 1 && key[0] >= '0' && key[0] <= '9':
		vk = uint32(key[0])
	case key[0] == 'F' && len(key) > 1:
		n, e := strconv.Atoi(key[1:])
		if e != nil || n < 1 || n > 24 {
			return 0, 0, fmt.Errorf("tecla desconhecida: %q", key)
		}
		vk = uint32(0x70 + n - 1)
	default:
		v, ok := named[key]
		if !ok {
			return 0, 0, fmt.Errorf("tecla desconhecida: %q", key)
		}
		vk = v
	}
	return mods | ModNoRepeat, vk, nil
}
