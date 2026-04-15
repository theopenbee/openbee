package model

type EnvConfig struct {
	ID        string `json:"id"         db:"id"`
	Scope     string `json:"scope"      db:"scope"`
	ScopeID   *string `json:"scope_id"   db:"scope_id"`
	Key       string `json:"key"        db:"key"`
	EncValue  string `json:"-"          db:"enc_value"`
	Masked    string `json:"masked"     db:"masked"`
	CreatedAt int64  `json:"created_at" db:"created_at"`
	UpdatedAt int64  `json:"updated_at" db:"updated_at"`
}
