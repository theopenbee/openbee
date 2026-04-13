package env

import "errors"

// ErrNotFound is returned when an env config does not exist.
var ErrNotFound = errors.New("env config not found")

// ErrValidation is returned for invalid input (bad scope, reserved key, missing scope_id).
var ErrValidation = errors.New("validation error")
