package i18n

import (
	"embed"
	"fmt"

	"gopkg.in/yaml.v3"
)

//go:embed locales/*.yaml
var localesFS embed.FS

// M 是全局翻译实例，在 main() 最早阶段通过 Load() 初始化，之后只读。
var M = &Messages{}

// SupportedLangs 列出所有支持的语言代码。
var SupportedLangs = []string{"zh", "en"}

// Load 加载指定语言的翻译文件并设置 M。
// 若语言不支持（文件不存在），静默 fallback 到 zh。
func Load(lang string) error {
	data, err := localesFS.ReadFile("locales/" + lang + ".yaml")
	if err != nil {
		data, err = localesFS.ReadFile("locales/zh.yaml")
		if err != nil {
			return fmt.Errorf("i18n: failed to load default locale: %w", err)
		}
	}
	m := &Messages{}
	if err := yaml.Unmarshal(data, m); err != nil {
		return fmt.Errorf("i18n: parse locale %q: %w", lang, err)
	}
	M = m
	return nil
}
