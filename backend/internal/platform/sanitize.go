package platform

import "regexp"

var sanitizeFileNameRe = regexp.MustCompile(`[\x00-\x1f\x7f\r\n"\\//:]+`)

// SanitizeFileName removes control characters and path separators for safe file handling (CWE-93).
func SanitizeFileName(name string) string {
	return sanitizeFileNameRe.ReplaceAllString(name, "_")
}
