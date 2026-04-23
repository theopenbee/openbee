package worker

import "errors"

var ErrNotFound = errors.New("worker not found")

var ErrValidation = errors.New("validation error")
