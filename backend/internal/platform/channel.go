package platform

type Channel struct {
	Platform       Platform
	ExtractContext func(raw string) string
}
