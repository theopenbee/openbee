package pluginmgr

import (
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/theopenbee/openbee/internal/infra/crypto"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/plugin"
)

type Settings struct {
	Enabled bool
	BotName string
	Config  map[string]any
}

type Codec struct {
	gcm        cipher.AEAD
	configured bool
}

func NewCodec(secret string, configured bool) (*Codec, error) {
	gcm, err := crypto.NewGCM(secret)
	if err != nil {
		return nil, err
	}
	return &Codec{gcm: gcm, configured: configured}, nil
}

func (c *Codec) Encode(s Settings, id string) (model.PluginSettings, error) {
	if !c.configured {
		return model.PluginSettings{}, errEnvSecretMissing()
	}
	cfg := s.Config
	if cfg == nil {
		cfg = map[string]any{}
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return model.PluginSettings{}, fmt.Errorf("encode plugin config: %w", err)
	}
	enc, err := crypto.EncryptGCM(c.gcm, string(data))
	if err != nil {
		return model.PluginSettings{}, fmt.Errorf("encrypt plugin config: %w", err)
	}
	return model.PluginSettings{ID: id, Enabled: s.Enabled, BotName: s.BotName, Config: enc}, nil
}

func (c *Codec) Decode(p model.PluginSettings) (Settings, error) {
	out := Settings{Enabled: p.Enabled, BotName: p.BotName, Config: map[string]any{}}
	if p.Config == "" {
		return out, nil
	}
	plain, err := crypto.DecryptGCM(c.gcm, p.Config)
	if err != nil {
		return Settings{}, fmt.Errorf("decrypt settings of plugin %q (was server.env_secret changed?): %w", p.ID, err)
	}
	if err := json.Unmarshal([]byte(plain), &out.Config); err != nil {
		return Settings{}, fmt.Errorf("decode settings of plugin %q: %w", p.ID, err)
	}
	return out, nil
}

func MergeConfig(m plugin.Manifest, current, update map[string]any) (map[string]any, error) {
	declared := make(map[string]bool, len(m.Config))
	for _, f := range m.Config {
		declared[f.Key] = true
	}
	for k := range update {
		if !declared[k] {
			return nil, errUnknownField(k)
		}
	}
	out := make(map[string]any, len(m.Config))
	for _, f := range m.Config {
		cur, hasCur := current[f.Key]
		v, sent := update[f.Key]
		switch {
		case sent && f.Type == plugin.FieldSecret && (v == nil || v == ""):
			if hasCur {
				out[f.Key] = cur
			}
		case sent && v == nil:
		case sent:
			out[f.Key] = v
		case hasCur:
			out[f.Key] = cur
		case f.RandomDefault:
			out[f.Key] = randomHex(8)
		}
	}
	if _, err := plugin.ResolveConfig(m, out); err != nil {
		return nil, errConfigInvalid(err)
	}
	return out, nil
}

func MaskConfig(m plugin.Manifest, cfg map[string]any) map[string]any {
	out := make(map[string]any, len(m.Config))
	for _, f := range m.Config {
		v, ok := cfg[f.Key]
		if !ok {
			continue
		}
		if s, isStr := v.(string); f.Type == plugin.FieldSecret && isStr && s != "" {
			v = crypto.Mask(s)
		}
		out[f.Key] = v
	}
	return out
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
