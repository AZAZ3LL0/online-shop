package auth_test

import (
	"errors"
	"testing"

	"github.com/qzq-kiim/shop/internal/auth"
)

func TestHashVerifyRoundTrip(t *testing.T) {
	hash, err := auth.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if err := auth.Verify("correct horse battery staple", hash); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestVerifyRejectsWrongPassword(t *testing.T) {
	hash, err := auth.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if err := auth.Verify("wrong", hash); !errors.Is(err, auth.ErrMismatch) {
		t.Fatalf("want ErrMismatch, got %v", err)
	}
}

func TestHashIsSalted(t *testing.T) {
	a, err := auth.Hash("same")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	b, err := auth.Hash("same")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if a == b {
		t.Fatal("two hashes of the same password must differ")
	}
}

func TestVerifyRejectsGarbageHash(t *testing.T) {
	if err := auth.Verify("x", "not-a-hash"); err == nil {
		t.Fatal("garbage hash must be rejected")
	}
}
