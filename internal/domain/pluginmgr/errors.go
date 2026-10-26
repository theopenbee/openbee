package pluginmgr

import (
	"fmt"

	"github.com/theopenbee/openbee/internal/apperr"
)

const (
	CodeNotFound           = "plugin_not_found"
	CodeConfigInvalid      = "plugin_config_invalid"
	CodeUnknownField       = "plugin_unknown_field"
	CodeEnvSecretMissing   = "plugin_env_secret_missing"
	CodeNotConfigured      = "plugin_not_configured"
	CodeBotNameConflict    = "plugin_bot_name_conflict"
	CodeOperationNotFound  = "plugin_operation_not_found"
	CodeCatalogUnavailable = "plugin_catalog_unavailable"
	CodeBusy               = "plugin_busy"
)

func errNotFound(id string) error {
	return apperr.New(CodeNotFound, fmt.Sprintf("plugin %q is not installed", id)).WithParams(map[string]any{"id": id})
}

func errConfigInvalid(err error) error {
	return apperr.New(CodeConfigInvalid, err.Error()).WithParams(map[string]any{"reason": err.Error()})
}

func errUnknownField(key string) error {
	return apperr.New(CodeUnknownField, fmt.Sprintf("config %q is not declared by the plugin", key)).WithParams(map[string]any{"key": key})
}

func errEnvSecretMissing() error {
	return apperr.New(CodeEnvSecretMissing, "server.env_secret is not set in config.yaml; set it before saving plugin settings")
}

func errNotConfigured(id string, err error) error {
	return apperr.New(CodeNotConfigured, fmt.Sprintf("plugin %q is not configured: %v", id, err)).WithParams(map[string]any{"id": id, "reason": err.Error()})
}

func errBotNameConflict(name string) error {
	return apperr.New(CodeBotNameConflict, fmt.Sprintf("bot name %q is already used by a worker", name)).WithParams(map[string]any{"name": name})
}

func errOperationNotFound(id string) error {
	return apperr.New(CodeOperationNotFound, fmt.Sprintf("operation %q not found", id)).WithParams(map[string]any{"id": id})
}

func errCatalogUnavailable(err error) error {
	return apperr.New(CodeCatalogUnavailable, fmt.Sprintf("plugin catalog is unavailable: %v", err)).WithParams(map[string]any{"reason": err.Error()})
}
