package kdf

import (
	"encoding/hex"
	"testing"
)

func TestPBKDF2Vector(t *testing.T) {
	// RFC 7914, seção 11
	got := hex.EncodeToString(PBKDF2([]byte("passwd"), []byte("salt"), 1, 64))
	want := "55ac046e56e3089fec1691c22544b605f94185216dde0465e68b9d57c20dacbc49ca9cccf179b645991664b39d77ef317c71b845b1e30bd509112041d3a19783"
	if got != want {
		t.Fatalf("pbkdf2 = %s", got)
	}
}
