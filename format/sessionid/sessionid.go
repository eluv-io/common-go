package sessionid

import (
	"crypto/rand"
	"fmt"
	"strconv"
)

// CreateSessionId creates a 12-character (6-byte hex encoded) session ID
func CreateSessionId() string {
	bts := make([]byte, 6)
	_, _ = rand.Read(bts)
	return fmt.Sprintf("%X", bts)
}

func IsSessionId(s string) bool {
	_, hexErr := strconv.ParseInt(s, 16, 64)
	return len(s) == 12 && hexErr == nil
}
