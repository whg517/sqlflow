package crypto

import "testing"

func TestValidateKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"16 bytes (AES-128)", "1234567890123456", false},
		{"24 bytes (AES-192)", "123456789012345678901234", false},
		{"32 bytes (AES-256)", "12345678901234567890123456789012", false},
		{"empty key", "", true},
		{"1 byte", "a", true},
		{"15 bytes", "123456789012345", true},
		{"17 bytes", "12345678901234567", true},
		{"33 bytes", "123456789012345678901234567890123", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateKey(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateKey(%q) error = %v, wantErr %v", tt.key, err, tt.wantErr)
			}
		})
	}
}

func TestEncryptDecrypt(t *testing.T) {
	key := "1234567890123456"
	plaintext := "hello world"

	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if ciphertext == plaintext {
		t.Error("ciphertext should differ from plaintext")
	}

	got, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if got != plaintext {
		t.Errorf("Decrypt() = %q, want %q", got, plaintext)
	}
}

func TestEncryptInvalidKey(t *testing.T) {
	_, err := Encrypt("test", "short")
	if err == nil {
		t.Error("expected error for invalid key length")
	}
}
