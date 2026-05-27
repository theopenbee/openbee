package platform

import (
	"errors"
	"regexp"
)

// AccountKey returns the composite routing key used to address a single
// account on a platform (e.g. "feishu:marketing-bot").
func AccountKey(platformID, accountName string) string {
	return platformID + ":" + accountName
}

var accountNameRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9_-]*[a-z0-9])?$`)

// ValidateAccountName enforces the lowercase [a-z0-9_-] alphabet,
// non-empty, no leading/trailing dash or underscore.
func ValidateAccountName(name string) error {
	if name == "" {
		return errors.New("account name must not be empty")
	}
	if !accountNameRE.MatchString(name) {
		return errors.New("account name must match [a-z0-9][a-z0-9_-]*[a-z0-9]")
	}
	return nil
}
