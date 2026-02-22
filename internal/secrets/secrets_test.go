package secrets

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	m := NewManager("test-key")

	cases := []struct {
		name      string
		plaintext string
	}{
		{"empty string", ""},
		{"short string", "hi"},
		{"typical string", "my-secret-api-key-12345"},
		{"long string", strings.Repeat("abcdefghij", 100)},
		{"special characters", "!@#$%^&*()_+-=[]{}|;':\",./<>?"},
		{"unicode", "Hello, \u4e16\u754c! \U0001f600 caf\u00e9 \u00fc\u00f1\u00ee\u00e7\u00f6d\u00e9"},
		{"newlines and tabs", "line1\nline2\ttab"},
		{"null bytes", "before\x00after"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			encrypted, err := m.Encrypt(tc.plaintext)
			if err != nil {
				t.Fatalf("Encrypt(%q) returned error: %v", tc.plaintext, err)
			}

			if encrypted == "" {
				t.Fatal("Encrypt returned empty string")
			}

			decrypted, err := m.Decrypt(encrypted)
			if err != nil {
				t.Fatalf("Decrypt returned error: %v", err)
			}

			if decrypted != tc.plaintext {
				t.Errorf("round-trip failed: got %q, want %q", decrypted, tc.plaintext)
			}
		})
	}
}

func TestEncryptProducesDifferentCiphertexts(t *testing.T) {
	m := NewManager("test-key")
	plaintext := "same-input-every-time"

	seen := make(map[string]bool)
	for i := 0; i < 20; i++ {
		encrypted, err := m.Encrypt(plaintext)
		if err != nil {
			t.Fatalf("Encrypt returned error on iteration %d: %v", i, err)
		}
		if seen[encrypted] {
			t.Fatalf("duplicate ciphertext on iteration %d: %s", i, encrypted)
		}
		seen[encrypted] = true
	}
}

func TestDecryptWithWrongKey(t *testing.T) {
	m1 := NewManager("key-one")
	m2 := NewManager("key-two")

	encrypted, err := m1.Encrypt("secret data")
	if err != nil {
		t.Fatalf("Encrypt returned error: %v", err)
	}

	_, err = m2.Decrypt(encrypted)
	if err == nil {
		t.Fatal("Decrypt with wrong key should have returned an error")
	}
}

func TestDecryptCorruptedCiphertext(t *testing.T) {
	m := NewManager("test-key")

	encrypted, err := m.Encrypt("hello")
	if err != nil {
		t.Fatalf("Encrypt returned error: %v", err)
	}

	// Decode, flip a byte in the ciphertext portion, re-encode
	data, err := hex.DecodeString(encrypted)
	if err != nil {
		t.Fatalf("hex.DecodeString returned error: %v", err)
	}

	// Flip the last byte (well within the ciphertext, past the nonce)
	data[len(data)-1] ^= 0xff
	corrupted := hex.EncodeToString(data)

	_, err = m.Decrypt(corrupted)
	if err == nil {
		t.Fatal("Decrypt with corrupted ciphertext should have returned an error")
	}
}

func TestDecryptInvalidHex(t *testing.T) {
	m := NewManager("test-key")

	_, err := m.Decrypt("not-valid-hex!!!")
	if err == nil {
		t.Fatal("Decrypt with invalid hex should have returned an error")
	}
}

func TestDecryptTooShortCiphertext(t *testing.T) {
	m := NewManager("test-key")

	// GCM nonce is 12 bytes; provide fewer than that
	short := hex.EncodeToString([]byte{0x01, 0x02, 0x03})
	_, err := m.Decrypt(short)
	if err == nil {
		t.Fatal("Decrypt with too-short ciphertext should have returned an error")
	}
	if !strings.Contains(err.Error(), "ciphertext too short") {
		t.Errorf("expected 'ciphertext too short' error, got: %v", err)
	}
}

func TestGenerateKeyFormat(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey returned error: %v", err)
	}

	if len(key) != 64 {
		t.Errorf("expected key length 64, got %d", len(key))
	}

	// Verify it is valid hex
	_, err = hex.DecodeString(key)
	if err != nil {
		t.Errorf("GenerateKey returned invalid hex: %v", err)
	}
}

func TestGenerateKeyUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 50; i++ {
		key, err := GenerateKey()
		if err != nil {
			t.Fatalf("GenerateKey returned error on iteration %d: %v", i, err)
		}
		if seen[key] {
			t.Fatalf("duplicate key on iteration %d: %s", i, key)
		}
		seen[key] = true
	}
}

func TestNewManagerDifferentKeysProduceDifferentResults(t *testing.T) {
	m1 := NewManager("alpha")
	m2 := NewManager("beta")
	plaintext := "same-plaintext"

	enc1, err := m1.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt with m1 returned error: %v", err)
	}

	enc2, err := m2.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt with m2 returned error: %v", err)
	}

	// While theoretically possible to collide, in practice
	// different keys with random nonces will never match
	if enc1 == enc2 {
		t.Error("different keys produced identical ciphertexts")
	}
}

func TestEncryptOutputIsHex(t *testing.T) {
	m := NewManager("test-key")

	encrypted, err := m.Encrypt("verify hex output")
	if err != nil {
		t.Fatalf("Encrypt returned error: %v", err)
	}

	_, err = hex.DecodeString(encrypted)
	if err != nil {
		t.Errorf("Encrypt output is not valid hex: %v", err)
	}
}

func TestDecryptEmptyString(t *testing.T) {
	m := NewManager("test-key")

	_, err := m.Decrypt("")
	if err == nil {
		t.Fatal("Decrypt with empty string should have returned an error")
	}
}
