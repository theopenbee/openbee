// Package apperr provides a typed error that carries a stable, machine-readable
// code (and optional interpolation params) alongside a human-readable message.
//
// The code lets the web frontend translate the error into the user's language
// via its i18n catalog, while the message remains a sensible fallback for any
// client that does not recognize the code.
package apperr

import "errors"

// Error is a business error with a stable code. The code is what the frontend
// maps to a localized string; Message is the (English) fallback.
type Error struct {
	Code    string
	Message string
	// Params carries interpolation values for the localized message
	// (e.g. {"scope": "worker"}). Optional.
	Params map[string]any
	// wrapped, when set, is returned by Unwrap so callers can keep using
	// errors.Is against existing sentinel errors (e.g. domain ErrValidation).
	wrapped error
}

func (e *Error) Error() string { return e.Message }

func (e *Error) Unwrap() error { return e.wrapped }

// New builds a coded error with the given stable code and fallback message.
func New(code, message string) *Error {
	return &Error{Code: code, Message: message}
}

// WithParams attaches interpolation params and returns the error for chaining.
func (e *Error) WithParams(params map[string]any) *Error {
	e.Params = params
	return e
}

// Wrapping sets an underlying error so errors.Is keeps matching it (used to
// preserve compatibility with domain sentinels like ErrValidation).
func (e *Error) Wrapping(err error) *Error {
	e.wrapped = err
	return e
}

// Code returns the stable code of err if it (or anything it wraps) is an
// *Error, or "" otherwise.
func Code(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

// Params returns the interpolation params of err if it (or anything it wraps)
// is an *Error, or nil otherwise.
func Params(err error) map[string]any {
	var e *Error
	if errors.As(err, &e) {
		return e.Params
	}
	return nil
}
