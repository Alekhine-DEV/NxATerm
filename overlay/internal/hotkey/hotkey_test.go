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
		{"Ctrl+'", ModControl | ModNoRepeat, 0xDE, false},
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

func TestCombo(t *testing.T) {
	c, err := ParseCombo("Cmd+Option+Shift+T")
	if err != nil || !c.Super || !c.Alt || !c.Shift || c.Ctrl || c.Key != "T" {
		t.Fatalf("combo: %+v %v", c, err)
	}
	if ks, ok := c.X11Keysym(); !ok || ks != 't' {
		t.Fatalf("keysym: %#x", ks)
	}
	if kc, ok := c.MacKeycode(); !ok || kc != 17 {
		t.Fatalf("mac: %d", kc)
	}
	for in, want := range map[string]uint32{"Ctrl+F5": 0xffc2, "Ctrl+Space": 0x20, "Alt+Enter": 0xff0d, "Alt+5": '5', "Ctrl+/": '/', "Ctrl+Left": 0xff51} {
		c, err := ParseCombo(in)
		if err != nil {
			t.Fatal(in, err)
		}
		if ks, ok := c.X11Keysym(); !ok || ks != want {
			t.Errorf("%s keysym %#x, queria %#x", in, ks, want)
		}
	}
	// todo nome aceito pelo Windows precisa existir no macOS, exceto os que o Mac não tem
	for _, k := range []string{"Ctrl+A", "Ctrl+Z", "Ctrl+0", "Ctrl+9", "Ctrl+F1", "Ctrl+F12", "Ctrl+Tab", "Ctrl+Esc", "Ctrl+`", "Ctrl+["} {
		c, _ := ParseCombo(k)
		if _, ok := c.MacKeycode(); !ok {
			t.Errorf("%s sem código no macOS", k)
		}
	}
	// letras e números: 36 teclas sem repetir código
	seen := map[uint32]string{}
	for r := 'A'; r <= 'Z'; r++ {
		c, _ := ParseCombo("Ctrl+" + string(r))
		kc, ok := c.MacKeycode()
		if !ok || seen[kc] != "" {
			t.Fatalf("%c: código %d repetido ou ausente (%s)", r, kc, seen[kc])
		}
		seen[kc] = string(r)
	}
	for r := '0'; r <= '9'; r++ {
		c, _ := ParseCombo("Ctrl+" + string(r))
		kc, ok := c.MacKeycode()
		if !ok || seen[kc] != "" {
			t.Fatalf("%c: código %d repetido ou ausente (%s)", r, kc, seen[kc])
		}
		seen[kc] = string(r)
	}
}
