package shortener_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/turbocalhj/MyGoProject/internal/shortener"
)

func TestGenerateCodeShape(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		code, err := shortener.GenerateCode()
		require.NoError(t, err)
		require.Len(t, code, shortener.CodeLength)
		for _, r := range code {
			require.True(t, strings.ContainsRune(shortener.CodeAlphabet, r), "unexpected rune %q in %q", r, code)
		}
		seen[code] = true
	}
	require.Greater(t, len(seen), 90, "codes should not repeat frequently")
}
