package model

// User is a human account that can log in to the web console.
type User struct {
	ID           string `json:"id" db:"id"`
	Username     string `json:"username" db:"username"`
	PasswordHash string `json:"-" db:"password_hash"`
	DisplayName  string `json:"display_name" db:"display_name"`
	Status       string `json:"status" db:"status"`
	CreatedBy    string `json:"created_by" db:"created_by"`
	CreatedAt    int64  `json:"created_at" db:"created_at"`
	UpdatedAt    int64  `json:"updated_at" db:"updated_at"`
}

// UserWithRoles is a user plus its assigned roles, for list/detail responses.
type UserWithRoles struct {
	User
	Roles []Role `json:"roles"`
}

// Role is an RBAC role.
type Role struct {
	ID          string `json:"id" db:"id"`
	Name        string `json:"name" db:"name"`
	Description string `json:"description" db:"description"`
	IsSystem    bool   `json:"is_system" db:"is_system"`
	CreatedAt   int64  `json:"created_at" db:"created_at"`
	UpdatedAt   int64  `json:"updated_at" db:"updated_at"`
}

// RoleWithPermissions is a role plus its permission keys.
type RoleWithPermissions struct {
	Role
	Permissions []string `json:"permissions"`
}

// User status constants.
const (
	UserStatusActive   = "active"
	UserStatusDisabled = "disabled"
)

// Seed system role IDs (inserted by migration v47).
const (
	RoleIDSuperAdmin = "sysrole_superadmin"
	RoleIDAdmin      = "sysrole_admin"
	RoleIDMember     = "sysrole_member"
)
