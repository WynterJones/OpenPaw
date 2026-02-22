package auth

import (
	"testing"
	"time"
)

func TestNewService(t *testing.T) {
	svc := NewService("test-secret")
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
	if string(svc.jwtSecret) != "test-secret" {
		t.Errorf("jwtSecret = %q, want %q", string(svc.jwtSecret), "test-secret")
	}
	if svc.tokenTTL != 24*time.Hour {
		t.Errorf("tokenTTL = %v, want %v", svc.tokenTTL, 24*time.Hour)
	}
}

func TestHashPasswordAndCheckPassword(t *testing.T) {
	svc := NewService("secret")
	password := "my-secure-password"

	hash, err := svc.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	if hash == "" {
		t.Fatal("HashPassword returned empty hash")
	}
	if hash == password {
		t.Fatal("HashPassword returned plaintext password")
	}

	// Correct password should succeed
	if err := svc.CheckPassword(hash, password); err != nil {
		t.Errorf("CheckPassword with correct password returned error: %v", err)
	}
}

func TestCheckPasswordWrongPassword(t *testing.T) {
	svc := NewService("secret")
	hash, err := svc.HashPassword("correct-password")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}

	if err := svc.CheckPassword(hash, "wrong-password"); err == nil {
		t.Error("CheckPassword with wrong password returned nil error, want error")
	}
}

func TestHashPasswordUniqueness(t *testing.T) {
	svc := NewService("secret")
	password := "same-password"

	hash1, err := svc.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword (1) returned error: %v", err)
	}
	hash2, err := svc.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword (2) returned error: %v", err)
	}

	if hash1 == hash2 {
		t.Error("two calls to HashPassword with same input produced identical hashes; bcrypt should use random salt")
	}

	// Both hashes should still verify against the original password
	if err := svc.CheckPassword(hash1, password); err != nil {
		t.Errorf("CheckPassword with hash1 failed: %v", err)
	}
	if err := svc.CheckPassword(hash2, password); err != nil {
		t.Errorf("CheckPassword with hash2 failed: %v", err)
	}
}

func TestGenerateTokenAndValidateToken(t *testing.T) {
	svc := NewService("my-jwt-secret")

	tokenStr, err := svc.GenerateToken("user-123", "alice")
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}
	if tokenStr == "" {
		t.Fatal("GenerateToken returned empty string")
	}

	claims, err := svc.ValidateToken(tokenStr)
	if err != nil {
		t.Fatalf("ValidateToken returned error: %v", err)
	}
	if claims.UserID != "user-123" {
		t.Errorf("claims.UserID = %q, want %q", claims.UserID, "user-123")
	}
	if claims.Username != "alice" {
		t.Errorf("claims.Username = %q, want %q", claims.Username, "alice")
	}
}

func TestGenerateTokenWithTTLClaims(t *testing.T) {
	svc := NewService("secret")
	ttl := 2 * time.Hour

	before := time.Now().Add(-time.Second)
	tokenStr, err := svc.GenerateTokenWithTTL("uid-456", "bob", ttl)
	if err != nil {
		t.Fatalf("GenerateTokenWithTTL returned error: %v", err)
	}
	after := time.Now().Add(time.Second)

	claims, err := svc.ValidateToken(tokenStr)
	if err != nil {
		t.Fatalf("ValidateToken returned error: %v", err)
	}

	if claims.UserID != "uid-456" {
		t.Errorf("claims.UserID = %q, want %q", claims.UserID, "uid-456")
	}
	if claims.Username != "bob" {
		t.Errorf("claims.Username = %q, want %q", claims.Username, "bob")
	}

	// IssuedAt should be approximately now
	if claims.IssuedAt == nil {
		t.Fatal("claims.IssuedAt is nil")
	}
	iat := claims.IssuedAt.Time
	if iat.Before(before) || iat.After(after) {
		t.Errorf("IssuedAt = %v, want between %v and %v", iat, before, after)
	}

	// ExpiresAt should be approximately now + ttl
	if claims.ExpiresAt == nil {
		t.Fatal("claims.ExpiresAt is nil")
	}
	exp := claims.ExpiresAt.Time
	expectedExpLow := before.Add(ttl)
	expectedExpHigh := after.Add(ttl)
	if exp.Before(expectedExpLow) || exp.After(expectedExpHigh) {
		t.Errorf("ExpiresAt = %v, want between %v and %v", exp, expectedExpLow, expectedExpHigh)
	}

	// NotBefore should be approximately now
	if claims.NotBefore == nil {
		t.Fatal("claims.NotBefore is nil")
	}
	nbf := claims.NotBefore.Time
	if nbf.Before(before) || nbf.After(after) {
		t.Errorf("NotBefore = %v, want between %v and %v", nbf, before, after)
	}
}

func TestGenerateTokenUsesDefaultTTL(t *testing.T) {
	svc := NewService("secret")

	before := time.Now().Add(-time.Second)
	tokenStr, err := svc.GenerateToken("uid", "user")
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}
	after := time.Now().Add(time.Second)

	claims, err := svc.ValidateToken(tokenStr)
	if err != nil {
		t.Fatalf("ValidateToken returned error: %v", err)
	}

	if claims.ExpiresAt == nil {
		t.Fatal("claims.ExpiresAt is nil")
	}
	exp := claims.ExpiresAt.Time
	expectedLow := before.Add(24 * time.Hour)
	expectedHigh := after.Add(24 * time.Hour)
	if exp.Before(expectedLow) || exp.After(expectedHigh) {
		t.Errorf("ExpiresAt = %v, want between %v and %v (24h from now)", exp, expectedLow, expectedHigh)
	}
}

func TestValidateTokenExpired(t *testing.T) {
	svc := NewService("secret")

	// Generate a token that expires in 1 millisecond
	tokenStr, err := svc.GenerateTokenWithTTL("uid", "user", time.Millisecond)
	if err != nil {
		t.Fatalf("GenerateTokenWithTTL returned error: %v", err)
	}

	// Wait for the token to expire
	time.Sleep(50 * time.Millisecond)

	_, err = svc.ValidateToken(tokenStr)
	if err == nil {
		t.Fatal("ValidateToken with expired token returned nil error, want error")
	}
	if err != ErrInvalidToken {
		t.Errorf("error = %v, want %v", err, ErrInvalidToken)
	}
}

func TestValidateTokenMalformed(t *testing.T) {
	svc := NewService("secret")

	cases := []struct {
		name  string
		token string
	}{
		{"empty string", ""},
		{"random garbage", "not-a-jwt-at-all"},
		{"three dots", "aaa.bbb.ccc"},
		{"truncated", "eyJhbGciOiJIUzI1NiJ9.eyJ1c2Vy"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.ValidateToken(tc.token)
			if err == nil {
				t.Error("ValidateToken returned nil error for malformed token")
			}
			if err != ErrInvalidToken {
				t.Errorf("error = %v, want %v", err, ErrInvalidToken)
			}
		})
	}
}

func TestValidateTokenWrongSecret(t *testing.T) {
	svc1 := NewService("secret-one")
	svc2 := NewService("secret-two")

	tokenStr, err := svc1.GenerateToken("uid", "user")
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	// Token signed with secret-one should not validate with secret-two
	_, err = svc2.ValidateToken(tokenStr)
	if err == nil {
		t.Fatal("ValidateToken with wrong secret returned nil error, want error")
	}
	if err != ErrInvalidToken {
		t.Errorf("error = %v, want %v", err, ErrInvalidToken)
	}
}
