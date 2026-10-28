package api

type LocalizedText map[string]string

type Config struct {
	Language       string   `json:"language"`
	EnabledEngines []string `json:"enabled_engines"`
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}
