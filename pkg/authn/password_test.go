package authn

import (
	"strings"
	"testing"
)

func TestHashPassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{
			name:     "valid password",
			password: "TestPassword123",
			wantErr:  false,
		},
		{
			name:     "empty password",
			password: "",
			wantErr:  false, // Hashing should work even for empty passwords
		},
		{
			name:     "long password",
			password: strings.Repeat("a", 200),
			wantErr:  false,
		},
		{
			name:     "unicode password",
			password: "كلمة_مرور_عربية123",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := HashPassword(tt.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("HashPassword() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Check hash format
				if !strings.HasPrefix(hash, "$argon2id$v=19$") {
					t.Errorf("HashPassword() hash format invalid: %s", hash)
				}

				// Check hash parts count
				parts := strings.Split(hash, "$")
				if len(parts) != 6 {
					t.Errorf("HashPassword() hash should have 6 parts, got %d", len(parts))
				}
			}
		})
	}
}

func TestVerifyPassword(t *testing.T) {
	password := "TestPassword123"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	tests := []struct {
		name     string
		password string
		hash     string
		wantErr  error
	}{
		{
			name:     "correct password",
			password: password,
			hash:     hash,
			wantErr:  nil,
		},
		{
			name:     "incorrect password",
			password: "WrongPassword",
			hash:     hash,
			wantErr:  ErrHashMismatch,
		},
		{
			name:     "empty password with correct hash",
			password: "",
			hash:     hash,
			wantErr:  ErrHashMismatch,
		},
		{
			name:     "invalid hash format",
			password: password,
			hash:     "invalid_hash",
			wantErr:  ErrInvalidHash,
		},
		{
			name:     "malformed hash - wrong parts count",
			password: password,
			hash:     "$argon2id$v=19$m=65536",
			wantErr:  ErrInvalidHash,
		},
		{
			name:     "malformed hash - wrong algorithm",
			password: password,
			hash:     "$bcrypt$v=19$m=65536,t=3,p=2$salt$hash",
			wantErr:  ErrInvalidHash,
		},
		{
			name:     "malformed hash - wrong version",
			password: password,
			hash:     "$argon2id$v=18$m=65536,t=3,p=2$salt$hash",
			wantErr:  ErrInvalidHash,
		},
		{
			name:     "malformed hash - invalid base64 salt",
			password: password,
			hash:     "$argon2id$v=19$m=65536,t=3,p=2$invalid_base64!@#$hash",
			wantErr:  ErrInvalidHash,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyPassword(tt.password, tt.hash)
			if err != tt.wantErr {
				t.Errorf("VerifyPassword() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHashPasswordUniqueness(t *testing.T) {
	password := "TestPassword123"

	// Generate multiple hashes for the same password
	hashes := make([]string, 10)
	for i := 0; i < 10; i++ {
		hash, err := HashPassword(password)
		if err != nil {
			t.Fatalf("Failed to hash password: %v", err)
		}
		hashes[i] = hash
	}

	// Verify all hashes are unique (due to random salt)
	for i := 0; i < len(hashes); i++ {
		for j := i + 1; j < len(hashes); j++ {
			if hashes[i] == hashes[j] {
				t.Errorf("Hashes should be unique, but found duplicate: %s", hashes[i])
			}
		}
	}

	// Verify all hashes can verify the same password
	for i, hash := range hashes {
		if err := VerifyPassword(password, hash); err != nil {
			t.Errorf("Hash %d failed verification: %v", i, err)
		}
	}
}

func TestIsValidPassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		want     bool
	}{
		{
			name:     "valid password with letters and numbers",
			password: "Password123",
			want:     true,
		},
		{
			name:     "valid password with special characters",
			password: "Password123!@#",
			want:     true,
		},
		{
			name:     "too short",
			password: "Pass1",
			want:     false,
		},
		{
			name:     "too long",
			password: strings.Repeat("a", 129) + "1",
			want:     false,
		},
		{
			name:     "no numbers",
			password: "PasswordOnly",
			want:     false,
		},
		{
			name:     "no letters",
			password: "12345678",
			want:     false,
		},
		{
			name:     "empty password",
			password: "",
			want:     false,
		},
		{
			name:     "only special characters",
			password: "!@#$%^&*()",
			want:     false,
		},
		{
			name:     "arabic with numbers",
			password: "كلمة_مرور123",
			want:     true,
		},
		{
			name:     "mixed case with numbers",
			password: "MyPassword123",
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidPassword(tt.password); got != tt.want {
				t.Errorf("IsValidPassword() = %v, want %v", got, tt.want)
			}
		})
	}
}

func BenchmarkHashPassword(b *testing.B) {
	password := "TestPassword123"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := HashPassword(password)
		if err != nil {
			b.Fatalf("HashPassword failed: %v", err)
		}
	}
}

func BenchmarkVerifyPassword(b *testing.B) {
	password := "TestPassword123"
	hash, err := HashPassword(password)
	if err != nil {
		b.Fatalf("Failed to hash password: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := VerifyPassword(password, hash)
		if err != nil {
			b.Fatalf("VerifyPassword failed: %v", err)
		}
	}
}

// TestPasswordSecurity tests that the password hashing is secure
func TestPasswordSecurity(t *testing.T) {
	password := "TestPassword123"

	// Test that same password produces different hashes
	hash1, err := HashPassword(password)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	hash2, err := HashPassword(password)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	if hash1 == hash2 {
		t.Error("Same password should produce different hashes due to random salt")
	}

	// Test that both hashes verify correctly
	if err := VerifyPassword(password, hash1); err != nil {
		t.Errorf("First hash failed verification: %v", err)
	}

	if err := VerifyPassword(password, hash2); err != nil {
		t.Errorf("Second hash failed verification: %v", err)
	}

	// Test that wrong password fails verification
	if err := VerifyPassword("WrongPassword", hash1); err != ErrHashMismatch {
		t.Errorf("Wrong password should fail verification with ErrHashMismatch, got: %v", err)
	}
}
