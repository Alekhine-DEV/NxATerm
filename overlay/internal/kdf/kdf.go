// Package kdf deriva chaves a partir de senhas.
package kdf

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
)

// PBKDF2 deriva uma chave com HMAC-SHA256 (RFC 8018).
func PBKDF2(password, salt []byte, iter, keyLen int) []byte {
	prf := hmac.New(sha256.New, password)
	hLen := prf.Size()
	blocks := (keyLen + hLen - 1) / hLen
	out := make([]byte, 0, blocks*hLen)
	u := make([]byte, hLen)
	var idx [4]byte
	for b := 1; b <= blocks; b++ {
		prf.Reset()
		prf.Write(salt)
		binary.BigEndian.PutUint32(idx[:], uint32(b))
		prf.Write(idx[:])
		t := prf.Sum(nil)
		copy(u, t)
		for i := 1; i < iter; i++ {
			prf.Reset()
			prf.Write(u)
			u = prf.Sum(u[:0])
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}
