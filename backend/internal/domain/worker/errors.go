package worker

import "errors"

var ErrNotFound = errors.New("worker not found")

var ErrValidation = errors.New("validation error")

type EngineModelsError struct {
	Err error
}

func (e *EngineModelsError) Error() string {
	return e.Err.Error()
}

func (e *EngineModelsError) Unwrap() error {
	return e.Err
}
