package hotkey

import "testing"

func TestParse(t *testing.T) {
	cases := []struct {
		in   string
		mods uint32
		vk   uint32
		bad  bool
	}{
		{"Ctrl+Alt+T", ModControl | ModAlt | ModNoRepeat, 'T', false},
		{"ctrl + shift + f12", ModControl | ModShift | ModNoRepeat, 0x7B, false},
		{"Win+`", ModWin | ModNoRepeat, 0xC0, false},
		{"Alt+Space", ModAlt | ModNoRepeat, 0x20, false},
		{"Ctrl+5", ModControl | ModNoRepeat, '5', false},
		{"T", 0, 0, true},
		{"Shift+T", 0, 0, true},
		{"Ctrl+", 0, 0, true},
		{"Ctrl+A+B", 0, 0, true},
		{"Ctrl+F99", 0, 0, true},
		{"", 0, 0, true},
	}
	for _, c := range cases {
		m, v, err := Parse(c.in)
		if c.bad {
			if err == nil {
				t.Errorf("%q devia falhar", c.in)
			}
			continue
		}
		if err != nil || m != c.mods || v != c.vk {
			t.Errorf("%q -> %#x %#x %v", c.in, m, v, err)
		}
	}
}
