package crypto

import (
	"strings"
	"testing"
)

func TestDumbDumbEncryption_ReferenceVectors(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		key      string
		expected string
	}{
		{
			name:     "CSharp Reference Vector 1",
			input:    "Hello, World!",
			key:      "secret_key",
			expected: "!enc:QgEfCwo8f1gKHg8AOw==",
		},
		{
			name:     "Empty String",
			input:    "",
			key:      "any_key",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encrypted := Encrypt(tt.input, tt.key)
			if encrypted != tt.expected {
				t.Errorf("Encrypt(%q, %q) = %q; want %q", tt.input, tt.key, encrypted, tt.expected)
			}

			decrypted := Decrypt(encrypted, tt.key)
			if decrypted != tt.input {
				t.Errorf("Decrypt(%q, %q) = %q; want %q", encrypted, tt.key, decrypted, tt.input)
			}
		})
	}
}

func TestDumbDumbEncryption_InvalidInput(t *testing.T) {
	input := "invalid_format"
	key := "any_key"

	decrypted := Decrypt(input, key)
	if decrypted != input {
		t.Errorf("Decrypt(%q, %q) = %q; want original %q", input, key, decrypted, input)
	}
}

func TestDumbDumbEncryption_Roundtrip(t *testing.T) {
	plaintext := "my_bnet_secret_12345"
	key := "TMP_CHANGE_ME"

	encrypted := Encrypt(plaintext, key)
	if !strings.HasPrefix(encrypted, Magic) {
		t.Fatalf("Expected prefix %q, got: %q", Magic, encrypted)
	}

	decrypted := Decrypt(encrypted, key)
	if decrypted != plaintext {
		t.Errorf("Decrypt(%q) = %q; want %q", encrypted, decrypted, plaintext)
	}
}
