package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
)

const (
	ManifestFile          = "openbee.plugin.json"
	ProtocolVersion       = 1
	EngineProtocolVersion = 1
	AvatarProtocolVersion = 1
)

const (
	TypeChannel = "channel"
	TypeEngine  = "engine"
	TypeAvatar  = "avatar"
)

var ReservedIDs = []string{"local"}

var reservedConfigKeys = []string{"enabled", "bot_name"}

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,31}$`)

type LocalizedText map[string]string

func (t *LocalizedText) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*t = LocalizedText{"en": s}
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("localized text must be a string or an object of strings")
	}
	*t = m
	return nil
}

func (t LocalizedText) Get(lang string) string {
	if v := t[lang]; v != "" {
		return v
	}
	return t["en"]
}

type FieldType string

const (
	FieldString FieldType = "string"
	FieldSecret FieldType = "secret"
	FieldBool   FieldType = "bool"
	FieldInt    FieldType = "int"
	FieldList   FieldType = "list"
)

var fieldTypes = []FieldType{FieldString, FieldSecret, FieldBool, FieldInt, FieldList}

type ConfigField struct {
	Key           string        `json:"key"`
	Type          FieldType     `json:"type"`
	Required      bool          `json:"required,omitempty"`
	Default       any           `json:"default,omitempty"`
	RandomDefault bool          `json:"random_default,omitempty"`
	Label         LocalizedText `json:"label,omitempty"`
	Help          LocalizedText `json:"help,omitempty"`
}

type Setup struct {
	Command []string      `json:"command"`
	Label   LocalizedText `json:"label,omitempty"`
}

type Manifest struct {
	ProtocolVersion int           `json:"protocol_version"`
	Type            string        `json:"type,omitempty"`
	ID              string        `json:"id"`
	Name            LocalizedText `json:"name,omitempty"`
	Description     LocalizedText `json:"description,omitempty"`
	Command         []string      `json:"command"`
	Setup           *Setup        `json:"setup,omitempty"`
	Config          []ConfigField `json:"config,omitempty"`
	Version         string        `json:"version,omitempty"`
	Icon            string        `json:"icon,omitempty"`
	Avatars         string        `json:"avatars,omitempty"`
	IconSVG         string        `json:"-"`
	Dir             string        `json:"-"`
}

func LoadManifest(dir string) (Manifest, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Manifest{}, fmt.Errorf("resolve plugin dir: %w", err)
	}
	data, err := os.ReadFile(filepath.Join(abs, ManifestFile))
	if err != nil {
		return Manifest{}, fmt.Errorf("read %s: %w", ManifestFile, err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("parse %s: %w", ManifestFile, err)
	}
	m.Dir = abs
	if v := packageVersion(abs); v != "" {
		m.Version = v
	}
	if err := m.Validate(); err != nil {
		return Manifest{}, err
	}
	if m.Icon != "" {
		if m.IconSVG, err = loadIcon(abs, m.Icon); err != nil {
			return Manifest{}, err
		}
	}
	return m, nil
}

func packageVersion(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return ""
	}
	var pkg struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return ""
	}
	return pkg.Version
}

func ValidateID(id string) error {
	if !idPattern.MatchString(id) {
		return fmt.Errorf("invalid plugin id %q: must match %s", id, idPattern.String())
	}
	if slices.Contains(ReservedIDs, id) {
		return fmt.Errorf("plugin id %q is reserved for a built-in channel", id)
	}
	return nil
}

func (m Manifest) Kind() string {
	if m.Type == "" {
		return TypeChannel
	}
	return m.Type
}

func (m Manifest) Validate() error {
	var supported int
	switch m.Kind() {
	case TypeChannel:
		supported = ProtocolVersion
	case TypeEngine:
		supported = EngineProtocolVersion
	case TypeAvatar:
		supported = AvatarProtocolVersion
	default:
		return fmt.Errorf("unknown type %q (supported: %s, %s, %s)", m.Type, TypeChannel, TypeEngine, TypeAvatar)
	}
	if m.ProtocolVersion != supported {
		return fmt.Errorf("unsupported protocol_version %d (supported: %d)", m.ProtocolVersion, supported)
	}
	if err := ValidateID(m.ID); err != nil {
		return err
	}
	if m.Kind() == TypeAvatar {
		return m.validateAvatar()
	}
	if m.Avatars != "" {
		return errors.New("only avatar plugins can declare avatars")
	}
	if len(m.Command) == 0 || m.Command[0] == "" {
		return errors.New("manifest command must not be empty")
	}
	if m.Kind() == TypeEngine && m.Setup != nil {
		return errors.New("engine plugins cannot declare setup")
	}
	if m.Setup != nil && (len(m.Setup.Command) == 0 || m.Setup.Command[0] == "") {
		return errors.New("manifest setup command must not be empty")
	}
	seen := make(map[string]bool, len(m.Config))
	for _, f := range m.Config {
		if f.Key == "" {
			return errors.New("config field key must not be empty")
		}
		if slices.Contains(reservedConfigKeys, f.Key) {
			return fmt.Errorf("config field key %q is reserved", f.Key)
		}
		if seen[f.Key] {
			return fmt.Errorf("duplicate config field key %q", f.Key)
		}
		seen[f.Key] = true
		if !slices.Contains(fieldTypes, f.Type) {
			return fmt.Errorf("config field %q has unknown type %q", f.Key, f.Type)
		}
		if f.RandomDefault && f.Type != FieldString {
			return fmt.Errorf("config field %q: random_default needs type %q", f.Key, FieldString)
		}
	}
	return nil
}

func (m Manifest) validateAvatar() error {
	switch {
	case len(m.Command) > 0:
		return errors.New("avatar plugins cannot declare command")
	case m.Setup != nil:
		return errors.New("avatar plugins cannot declare setup")
	case len(m.Config) > 0:
		return errors.New("avatar plugins cannot declare config")
	case m.Avatars == "":
		return errors.New("avatar plugins must declare avatars")
	case !filepath.IsLocal(filepath.FromSlash(m.Avatars)):
		return fmt.Errorf("avatars %q must be a relative path to a folder inside the plugin", m.Avatars)
	}
	return nil
}

func (m Manifest) DisplayName(lang string) string {
	if n := m.Name.Get(lang); n != "" {
		return n
	}
	return m.ID
}
