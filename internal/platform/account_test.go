package platform

import "testing"

func TestAccountKey(t *testing.T) {
	if got := AccountKey("feishu", "marketing-bot"); got != "feishu:marketing-bot" {
		t.Fatalf("got %q", got)
	}
}

func TestValidateAccountName(t *testing.T) {
	good := []string{"default", "marketing-bot", "a", "team_1", "ops-2026"}
	for _, n := range good {
		if err := ValidateAccountName(n); err != nil {
			t.Fatalf("valid name %q rejected: %v", n, err)
		}
	}
	bad := []string{"", "Marketing", "bot.1", "a b", "你好", "-leading", "trailing-"}
	for _, n := range bad {
		if err := ValidateAccountName(n); err == nil {
			t.Fatalf("invalid name %q accepted", n)
		}
	}
}
