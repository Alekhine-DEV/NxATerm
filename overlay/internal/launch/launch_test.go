package launch

import (
	"reflect"
	"testing"
)

func TestCollect(t *testing.T) {
	got := Collect([]string{"ssh://ana@srv:2222", "--session", "Produção", "x", "telnet://sw", "--session=Casa", "-s"})
	want := []Item{"ssh://ana@srv:2222", "session:Produção", "telnet://sw", "session:Casa"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%v", got)
	}
	if len(Collect(nil)) != 0 {
		t.Fatal("sem argumentos deveria vir vazio")
	}
}

func TestFromURL(t *testing.T) {
	got, err := FromURL("ssh://ana@10.0.0.5:2222/")
	if err != nil || got != (Target{"ssh", "ana", "10.0.0.5", 2222}) {
		t.Fatalf("%+v %v", got, err)
	}
	got, err = FromURL("telnet://switch.local")
	if err != nil || got != (Target{"telnet", "", "switch.local", 0}) {
		t.Fatalf("%+v %v", got, err)
	}
	got, err = FromURL("SSH://root@[::1]:22")
	if err != nil || got.Host != "::1" || got.Port != 22 {
		t.Fatalf("%+v %v", got, err)
	}
	for _, bad := range []string{"ssh://host", "http://x", "ssh://u@h:0", "ssh://u@:22", "ssh://u@h:abc"} {
		if _, err := FromURL(bad); err == nil {
			t.Errorf("%q deveria falhar", bad)
		}
	}
}
