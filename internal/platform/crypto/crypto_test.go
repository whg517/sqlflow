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

func TestDecryptInvalidBase64(t *testing.T) {
	_, err := Decrypt("not-valid-base64!!!", "1234567890123456")
	if err == nil {
		t.Error("expected error for invalid base64 ciphertext")
	}
}

func TestDecryptWrongKey(t *testing.T) {
	ciphertext, err := Encrypt("secret data", "1234567890123456")
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	_, err = Decrypt(ciphertext, "abcdefghijklmnop")
	if err == nil {
		t.Error("expected error when decrypting with wrong key")
	}
}

func TestDecryptTruncatedCiphertext(t *testing.T) {
	_, err := Decrypt("AQID", "1234567890123456")
	if err == nil {
		t.Error("expected error for ciphertext too short")
	}
}

func TestEncryptDecryptEmptyPlaintext(t *testing.T) {
	key := "1234567890123456"
	ciphertext, err := Encrypt("", key)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	got, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if got != "" {
		t.Errorf("Decrypt() = %q, want empty string", got)
	}
}

func TestEncryptDecryptUnicode(t *testing.T) {
	key := "1234567890123456"
	plaintext := "你好世界 🌍 こんにちは"
	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	got, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if got != plaintext {
		t.Errorf("Decrypt() = %q, want %q", got, plaintext)
	}
}

func TestEncryptDecryptAES192(t *testing.T) {
	key := "123456789012345678901234"
	plaintext := "AES-192 test"
	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	got, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if got != plaintext {
		t.Errorf("Decrypt() = %q, want %q", got, plaintext)
	}
}

func TestEncryptDecryptAES256(t *testing.T) {
	key := "12345678901234567890123456789012"
	plaintext := "AES-256 test"
	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	got, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if got != plaintext {
		t.Errorf("Decrypt() = %q, want %q", got, plaintext)
	}
}

func TestDecryptInvalidKey(t *testing.T) {
	_, err := Decrypt("dGVzdA==", "short")
	if err == nil {
		t.Error("expected error for invalid key length on Decrypt")
	}
}

func TestEncryptProducesDifferentCiphertexts(t *testing.T) {
	key := "1234567890123456"
	plaintext := "same input"
	c1, _ := Encrypt(plaintext, key)
	c2, _ := Encrypt(plaintext, key)
	if c1 == c2 {
		t.Error("two Encrypt calls with same input should produce different ciphertexts (random nonce)")
	}
}

func TestEncryptDecryptLongPlaintext(t *testing.T) {
	key := "1234567890123456"
	plaintext := string(make([]byte, 4096))
	for i := range plaintext {
		plaintext = plaintext[:i] + string(rune('A'+i%26)) + plaintext[i+1:]
	}
	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	got, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if got != plaintext {
		t.Error("Decrypt() mismatch for long plaintext")
	}
}

func TestEncryptDecryptSpecialCharacters(t *testing.T) {
	key := "1234567890123456"
	tests := []struct {
		name      string
		plaintext string
	}{
		{"null_bytes", "before\x00after"},
		{"newlines", "line1\nline2\rline3\r\n"},
		{"tabs", "col1\tcol2\tcol3"},
		{"quotes", `he said "hello" and she said 'bye'`},
		{"backslashes", `path\to\file\\end`},
		{"mixed_control_chars", "\x01\x02\x07\x1b\x7f"},
		{"all_special", "null\x00tab\tnewline\nquote\"single'backslash\\end"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ciphertext, err := Encrypt(tt.plaintext, key)
			if err != nil {
				t.Fatalf("Encrypt() error = %v", err)
			}
			got, err := Decrypt(ciphertext, key)
			if err != nil {
				t.Fatalf("Decrypt() error = %v", err)
			}
			if got != tt.plaintext {
				t.Errorf("Decrypt() = %q, want %q", got, tt.plaintext)
			}
		})
	}
}

func TestEncryptDecryptVeryLongInput(t *testing.T) {
	key := "1234567890123456"
	// 100KB plaintext
	b := make([]byte, 100*1024)
	for i := range b {
		b[i] = byte(i % 256)
	}
	plaintext := string(b)

	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	got, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if got != plaintext {
		t.Error("Decrypt() mismatch for very long input")
	}
}

func TestEncryptDecryptWhitespaceOnly(t *testing.T) {
	key := "1234567890123456"
	tests := []struct {
		name      string
		plaintext string
	}{
		{"spaces", "   "},
		{"tabs_only", "\t\t\t"},
		{"newlines_only", "\n\n"},
		{"mixed_whitespace", " \t\n\r "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ciphertext, err := Encrypt(tt.plaintext, key)
			if err != nil {
				t.Fatalf("Encrypt() error = %v", err)
			}
			got, err := Decrypt(ciphertext, key)
			if err != nil {
				t.Fatalf("Decrypt() error = %v", err)
			}
			if got != tt.plaintext {
				t.Errorf("Decrypt() = %q, want %q", got, tt.plaintext)
			}
		})
	}
}
