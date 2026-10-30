package model

// User is a human account that can log in to the web console.
type User struct {
	ID           string `json:"id" db:"id"`
	Username     string `json:"username" db:"username"`
	PasswordHash string `json:"-" db:"password_hash"`
	DisplayName  string `json:"display_name" db:"display_name"`
	CreatedAt    int64  `json:"created_at" db:"created_at"`
	UpdatedAt    int64  `json:"updated_at" db:"updated_at"`
	// PasswordChangedAt is the millisecond timestamp (floored to the second) of
	// the last password change. Access/refresh tokens issued before this instant
	// are rejected, forcing a re-login after any password change.
	PasswordChangedAt int64 `json:"-" db:"password_changed_at"`
}
