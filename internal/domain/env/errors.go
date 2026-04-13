package env

import "errors"

// ErrNotFound is returned when an env config does not exist.
var ErrNotFound = errors.New("env config not found")
