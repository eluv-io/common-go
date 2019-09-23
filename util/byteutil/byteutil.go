package byteutil

import (
	"math/rand"
)

func RandomBytes(length int) []byte {
	b := make([]byte, length)
	rand.Read(b)
	return b
}
