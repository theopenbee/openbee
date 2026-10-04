package utils

import (
	"net/http"
	"time"
)

// APIClient is the shared HTTP client for short API/version-check calls.
var APIClient = &http.Client{Timeout: 15 * time.Second}
