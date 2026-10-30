package main

import (
	"reflect"
	"testing"
)

func TestSplitCommand(t *testing.T) {
	cases := map[string][]string{
		"":                     nil,
		"cmd.exe":              {"cmd.exe"},
		"  pwsh.exe  -NoLogo ": {"pwsh.exe", "-NoLogo"},
		`"C:\Program Files\Git\bin\bash.exe" --login -i`: {`C:\Program Files\Git\bin\bash.exe`, "--login", "-i"},
		`a "b c" d`: {"a", "b c", "d"},
	}
	for in, want := range cases {
		if got := splitCommand(in); !reflect.DeepEqual(got, want) {
			t.Errorf("splitCommand(%q) = %q, esperado %q", in, got, want)
		}
	}
}

func TestUserDistros(t *testing.T) {
	got := userDistros([]string{"Arch", "docker-desktop", "Docker-Desktop-Data", "Ubuntu", "rancher-desktop"})
	if !reflect.DeepEqual(got, []string{"Arch", "Ubuntu"}) {
		t.Errorf("got %q", got)
	}
	if got := userDistros([]string{"docker-desktop"}); got == nil || len(got) != 0 {
		t.Errorf("só interna: %#v", got)
	}
}
