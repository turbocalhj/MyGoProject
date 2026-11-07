package shortener

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
)

const (
	// CodeLength is the length of auto-generated codes.
	CodeLength = 7
	// CodeAlphabet is the base62 alphabet used for codes.
	CodeAlphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
)

// GenerateCode returns a cryptographically random base62 code.
func GenerateCode() (string, error) {
	max := big.NewInt(int64(len(CodeAlphabet)))
	var b strings.Builder
	b.Grow(CodeLength)
	for i := 0; i < CodeLength; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("generate code: %w", err)
		}
		b.WriteByte(CodeAlphabet[n.Int64()])
	}
	return b.String(), nil
}
