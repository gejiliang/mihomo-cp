package service

import (
	"testing"
)

func TestGenerateAndValidateToken(t *testing.T) {
	svc := NewAuthService("test-secret-key", 1, 24)

	t.Run("access token", func(t *testing.T) {
		token, err := svc.GenerateAccessToken("user-123", "admin")
		if err != nil {
			t.Fatalf("GenerateAccessToken error: %v", err)
		}

		claims, err := svc.ValidateToken(token)
		if err != nil {
			t.Fatalf("ValidateToken error: %v", err)
		}

		if claims.UserID != "user-123" {
			t.Errorf("expected UserID=user-123, got %s", claims.UserID)
		}
		if claims.Role != "admin" {
			t.Errorf("expected Role=admin, got %s", claims.Role)
		}
		if claims.Type != "access" {
			t.Errorf("expected Type=access, got %s", claims.Type)
		}
	})

	t.Run("refresh token", func(t *testing.T) {
		token, err := svc.GenerateRefreshToken("user-456", "readonly")
		if err != nil {
			t.Fatalf("GenerateRefreshToken error: %v", err)
		}

		claims, err := svc.ValidateToken(token)
		if err != nil {
			t.Fatalf("ValidateToken error: %v", err)
		}

		if claims.UserID != "user-456" {
			t.Errorf("expected UserID=user-456, got %s", claims.UserID)
		}
		if claims.Role != "readonly" {
			t.Errorf("expected Role=readonly, got %s", claims.Role)
		}
		if claims.Type != "refresh" {
			t.Errorf("expected Type=refresh, got %s", claims.Type)
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		_, err := svc.ValidateToken("not.a.valid.token")
		if err == nil {
			t.Error("expected error for invalid token, got nil")
		}
	})

	t.Run("token signed with different secret", func(t *testing.T) {
		other := NewAuthService("other-secret", 1, 24)
		token, err := other.GenerateAccessToken("user-789", "admin")
		if err != nil {
			t.Fatalf("GenerateAccessToken error: %v", err)
		}
		_, err = svc.ValidateToken(token)
		if err == nil {
			t.Error("expected error for token with wrong secret, got nil")
		}
	})
}

func TestHashAndVerifyPassword(t *testing.T) {
	svc := NewAuthService("test-secret-key", 1, 24)

	t.Run("match", func(t *testing.T) {
		hash, err := svc.HashPassword("secret123")
		if err != nil {
			t.Fatalf("HashPassword error: %v", err)
		}
		if !svc.VerifyPassword(hash, "secret123") {
			t.Error("expected VerifyPassword to return true for matching password")
		}
	})

	t.Run("mismatch", func(t *testing.T) {
		hash, err := svc.HashPassword("secret123")
		if err != nil {
			t.Fatalf("HashPassword error: %v", err)
		}
		if svc.VerifyPassword(hash, "wrong-password") {
			t.Error("expected VerifyPassword to return false for wrong password")
		}
	})
}
