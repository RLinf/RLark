package storage

import (
	"bytes"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Vectors copied from rclone v1.71.2 fs/config/obscure/obscure_test.go so the
// gateway output stays compatible with the rclone CSI driver.
func TestRcloneObscureMatchesRcloneVectors(t *testing.T) {
	for _, test := range []struct{ in, want, iv string }{
		{"", "YWFhYWFhYWFhYWFhYWFhYQ", "aaaaaaaaaaaaaaaa"},
		{"potato", "YWFhYWFhYWFhYWFhYWFhYXMaGgIlEQ", "aaaaaaaaaaaaaaaa"},
		{"potato", "YmJiYmJiYmJiYmJiYmJiYp3gcEWbAw", "bbbbbbbbbbbbbbbb"},
	} {
		obscureRand = bytes.NewBufferString(test.iv)
		got, err := RcloneObscure(test.in)
		obscureRand = rand.Reader
		require.NoError(t, err)
		assert.Equal(t, test.want, got)
	}
}

func TestRcloneObscureUsesRandomIV(t *testing.T) {
	first, err := RcloneObscure("p@ss:word=with;symbols")
	require.NoError(t, err)
	second, err := RcloneObscure("p@ss:word=with;symbols")
	require.NoError(t, err)
	assert.NotEqual(t, first, second)
}
