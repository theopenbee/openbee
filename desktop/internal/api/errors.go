package api

type Error struct {
	Status  int
	Message string
	Code    string
	Params  map[string]any
	Details map[string]any
}

func (e *Error) Error() string {
	return e.Message
}
