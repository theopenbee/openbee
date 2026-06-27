package auth

import "testing"

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("s3cret")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "s3cret" || hash == "" {
		t.Fatal("hash must not equal plaintext or be empty")
	}
	if !CheckPassword(hash, "s3cret") {
		t.Fatal("expected correct password to match")
	}
	if CheckPassword(hash, "wrong") {
		t.Fatal("expected wrong password to fail")
	}
}
