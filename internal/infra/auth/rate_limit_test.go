package auth

import (
	"testing"
	"time"
)

func TestLoginRateLimiter_BlocksAfterBurst(t *testing.T) {
	l := NewLoginRateLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !l.Allow("1.2.3.4") {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
	}
	if l.Allow("1.2.3.4") {
		t.Fatal("attempt beyond burst should be blocked")
	}
}

func TestLoginRateLimiter_ResetClears(t *testing.T) {
	l := NewLoginRateLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		l.Allow("1.2.3.4")
	}
	if l.Allow("1.2.3.4") {
		t.Fatal("precondition: should be blocked before reset")
	}
	l.Reset("1.2.3.4")
	if !l.Allow("1.2.3.4") {
		t.Fatal("after reset the IP should be allowed again")
	}
}

func TestLoginRateLimiter_PerIPIsolation(t *testing.T) {
	l := NewLoginRateLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		l.Allow("1.1.1.1")
	}
	if !l.Allow("2.2.2.2") {
		t.Fatal("a different IP must not be affected by another IP's attempts")
	}
}
