package model

type PluginSettings struct {
	ID        string `db:"id"`
	Enabled   bool   `db:"enabled"`
	BotName   string `db:"bot_name"`
	Config    string `db:"config"`
	UpdatedAt int64  `db:"updated_at"`
}
