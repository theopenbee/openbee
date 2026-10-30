package cli

// ExitCodeError is a sentinel error that carries a non-zero exit code without
// printing an error message. Commands return this when they need to signal
// failure cleanly (e.g. "status" when the daemon is not running).
type ExitCodeError struct{ Code int }

func (e *ExitCodeError) Error() string { return "" }
