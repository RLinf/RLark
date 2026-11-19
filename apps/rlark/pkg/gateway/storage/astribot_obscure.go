package storage

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

// rcloneObscureKey is the fixed, publicly documented key rclone uses for
// "obscured" config values (fs/config/obscure). Obscuring is not encryption:
// it only keeps passwords from being readable at a glance in rclone configs.
// The value must stay byte-for-byte identical to rclone or the CSI driver will
// fail to reveal the password.
var rcloneObscureKey = []byte{
	0x9c, 0x93, 0x5b, 0x48, 0x73, 0x0a, 0x55, 0x4d,
	0x6b, 0xfd, 0x7c, 0x63, 0xc8, 0x86, 0xa9, 0x2b,
	0xd3, 0x90, 0x19, 0x8e, 0xb8, 0x12, 0x8a, 0xfb,
	0xf4, 0xde, 0x16, 0x2b, 0x8b, 0x95, 0xf6, 0x38,
}

// obscureRand supplies the IV; tests replace it to reproduce rclone vectors.
var obscureRand io.Reader = rand.Reader

func rcloneCrypt(out, in, iv []byte) error {
	block, err := aes.NewCipher(rcloneObscureKey)
	if err != nil {
		return err
	}
	cipher.NewCTR(block, iv).XORKeyStream(out, in)
	return nil
}

// RcloneObscure returns value in rclone's obscured form, as required for
// password fields such as the smb backend's "pass".
func RcloneObscure(value string) (string, error) {
	plaintext := []byte(value)
	ciphertext := make([]byte, aes.BlockSize+len(plaintext))
	iv := ciphertext[:aes.BlockSize]
	if _, err := io.ReadFull(obscureRand, iv); err != nil {
		return "", fmt.Errorf("read iv: %w", err)
	}
	if err := rcloneCrypt(ciphertext[aes.BlockSize:], plaintext, iv); err != nil {
		return "", fmt.Errorf("obscure: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(ciphertext), nil
}
