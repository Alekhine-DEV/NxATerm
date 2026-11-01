package store

import "testing"

func TestRemoteTypes(t *testing.T) {
	r, err := Normalize(Session{Type: "rdp", Host: " srv ", User: "DOM\\ana"})
	if err != nil || r.Port != 3389 || r.Name != "DOM\\ana@srv" {
		t.Fatalf("rdp: %+v %v", r, err)
	}
	v, err := Normalize(Session{Type: "vnc", Host: "h", User: "x"})
	if err != nil || v.Port != 5900 || v.User != "" || v.Name != "h" {
		t.Fatalf("vnc: %+v %v", v, err)
	}
	vj, _ := Normalize(Session{Type: "vnc", Host: "h", JumpHost: "abc", Proxy: "socks5://p:1"})
	if vj.JumpHost != "abc" || vj.Proxy != "socks5://p:1" {
		t.Fatalf("vnc deveria manter salto e proxy: %+v", vj)
	}
	rj, _ := Normalize(Session{Type: "rdp", Host: "h", JumpHost: "abc", Proxy: "socks5://p:1"})
	if rj.JumpHost != "" || rj.Proxy != "" {
		t.Fatalf("rdp não passa pelo app: %+v", rj)
	}
	if _, err := Normalize(Session{Type: "spice", Host: ""}); err == nil {
		t.Fatal("spice sem host deveria falhar")
	}
	w, err := Normalize(Session{Type: "web", URL: "exemplo.com/painel"})
	if err != nil || w.URL != "https://exemplo.com/painel" || w.Name != "exemplo.com" || w.Host != "" {
		t.Fatalf("web: %+v %v", w, err)
	}
	for _, bad := range []string{"", "ftp://x.com", "javascript:alert(1)", "https://u:p@x.com", "http://"} {
		if _, err := CleanWebURL(bad); err == nil {
			t.Fatalf("%q deveria falhar", bad)
		}
	}
	if key(w) != "web|https://exemplo.com/painel" {
		t.Fatal(key(w))
	}
}
