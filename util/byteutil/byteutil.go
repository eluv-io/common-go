package byteutil

import (
	"math/rand"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func RandomBytes(length int) []byte {
	b := make([]byte, length)
	_, _ = rand.Read(b)
	return b
}
