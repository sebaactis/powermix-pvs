package logging

import (
	"crypto/rand"
	"fmt"
	"time"
)


const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

func encodeCrockford(b []byte) string {
	var sb []byte
	for i := 0; i < len(b); i += 5 {
		// Tomar hasta 5 bytes (40 bits) y codificar a 8 caracteres Crockford.
		var buf uint64
		n := len(b) - i
		if n > 5 {
			n = 5
		}
		for j := 0; j < n; j++ {
			buf = (buf << 8) | uint64(b[i+j])
		}
		buf <<= uint((5 - n) * 8) // alinear a la izquierda

		chars := 8
		if n < 5 {
			chars = (n*8 + 4) / 5 // bits reales / 5, redondeado arriba
		}
		for k := chars - 1; k >= 0; k-- {
			sb = append(sb, crockford[(buf>>(uint(k)*5))&0x1F])
		}
	}
	return string(sb)
}


// newULID genera un ULID Crockford de 26 caracteres.
func newULID() string {
	var b [16]byte
	now := uint64(time.Now().UnixMilli())
	b[0] = byte(now >> 40)
	b[1] = byte(now >> 32)
	b[2] = byte(now >> 24)
	b[3] = byte(now >> 16)
	b[4] = byte(now >> 8)
	b[5] = byte(now)
	if _, err := rand.Read(b[6:16]); err != nil {
		panic("logging: fallo crypto/rand en ULID: " + err.Error())
	}
	return encodeCrockford(b[:])
}


func newUUIDv4() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("logging: fallo crypto/rand en UUID: " + err.Error())
	}
	b[6] = (b[6] & 0x0F) | 0x40 // versión 4
	b[8] = (b[8] & 0x3F) | 0x80 // variante RFC 4122
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}


func NewRequestID() string {
	return "req_" + newULID()
}

func NewScanID() string {
	return "scan_" + newUUIDv4()
}
