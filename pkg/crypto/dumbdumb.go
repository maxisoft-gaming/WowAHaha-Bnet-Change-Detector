package crypto

import (
	"encoding/base64"
	"strings"
)

const Magic = "!enc:"

// Encrypt encrypts a plain string value with key using the DumbDumb XOR+Reverse algorithm.
// If input is empty, returns value as-is.
func Encrypt(value, key string) string {
	if value == "" {
		return value
	}

	keyBytes := []byte(key)
	if len(keyBytes) == 0 {
		return value
	}

	valueBytes := []byte(value)
	for i := 0; i < len(valueBytes); i++ {
		valueBytes[i] ^= keyBytes[i%len(keyBytes)]
	}

	reverseBytes(valueBytes)

	return Magic + base64.StdEncoding.EncodeToString(valueBytes)
}

// Decrypt decrypts a string encrypted with Encrypt if it starts with "!enc:".
// If value does not start with "!enc:" or input is empty, returns value as-is.
func Decrypt(value, key string) string {
	if value == "" || !strings.HasPrefix(value, Magic) {
		return value
	}

	keyBytes := []byte(key)
	if len(keyBytes) == 0 {
		return value
	}

	encoded := strings.TrimPrefix(value, Magic)
	valueBytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return value
	}

	reverseBytes(valueBytes)

	for i := 0; i < len(valueBytes); i++ {
		valueBytes[i] ^= keyBytes[i%len(keyBytes)]
	}

	return string(valueBytes)
}

// DecryptIfNeeded inspects value: if it starts with "!enc:", decrypts it using key; otherwise returns value.
func DecryptIfNeeded(value, key string) string {
	return Decrypt(value, key)
}

func reverseBytes(b []byte) {
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
}
