# Avatar plugin protocol (v1)

An avatar plugin gives workers image avatars. It is a folder of images and a manifest: openbee runs no process for it, so it needs no runtime and can be written without code.

## Package layout

```
openbee.plugin.json
icon.svg
avatars/
  00001.webp
  00042.webp
  ...
```

```json
{
  "protocol_version": 1,
  "type": "avatar",
  "id": "mfers",
  "name": { "en": "mfers", "zh": "mfers" },
  "description": { "en": "300 mfers by sartoshi as worker avatars (CC0)" },
  "icon": "icon.svg",
  "avatars": "avatars"
}
```

| Field | Rules |
|---|---|
| `protocol_version` | Must be `1`. Versioned separately from the channel and engine protocols. |
| `type` | Must be `avatar`. |
| `id` | Matches `^[a-z][a-z0-9_-]{1,31}$`. Cannot be `local`. Shares one id space with channel and engine plugins. |
| `name`, `description` | As for channel plugins. `name` heads the plugin's avatars in the avatar picker when several avatar plugins are installed. |
| `avatars` | Required. Relative path of the image folder inside the plugin. |
| `icon` | Optional, as for [engine plugins](engine-plugin-protocol.md#icon). |
| `version` | Optional. `package.json` `version` wins when present. |
| `command`, `setup`, `config` | Not allowed; a manifest that declares any of them is rejected. |

### The image folder

Every file in the folder is an avatar, except names starting with `.`. If one file breaks a rule, the whole plugin fails to load and shows the error on its plugin page.

| Rule | |
|---|---|
| Format | `.webp`, `.png`, `.jpg` or `.jpeg`. Square images look best; they are shown in a circle from 28 to 88 px wide, so 256 × 256 is plenty. |
| Name | The file name without its extension matches `^[a-z0-9][a-z0-9_-]{0,63}$` and is unique in the folder. |
| Size | At most 256 KB per file, at most 2000 files. |
| Files | Regular files only, no symlinks or subfolders. |

Renaming or removing a file later drops it from the workers that use it; they show their initials instead. The folder may be a symlink, as long as it resolves to a folder inside the plugin.

## Lifecycle

An avatar plugin is enabled as soon as it is installed and shows as `ready`. `disable`, `enable`, `restart` (reads the folder again), `update` and `remove` work as for other plugins.

- A new worker gets a random avatar from the enabled avatar plugins, one no other worker uses while there are any left. Existing workers keep their initials.
- Editing a worker can pick another avatar, a random one, or go back to initials.
- A worker stores its avatar as `<plugin id>/<file name>`, for example `mfers/00042`. While the plugin is disabled or removed, the worker shows its initials; the stored avatar comes back when the plugin does.

## HTTP API

| Request | |
|---|---|
| `GET /api/avatars` | `{"packs": [{"id": "mfers", "name": {"en": "mfers"}, "avatars": ["mfers/00001", "mfers/00042"]}]}`, enabled plugins only. |
| `GET /api/avatars/<plugin id>/<name>` | The image. `404` with code `avatar_not_found` when it is not available. |
| `POST /api/workers` | Optional `avatar`. Left out or empty, the worker gets a random one. |
| `PUT /api/workers/<id>` | `"avatar": "mfers/00042"` sets it, `"avatar": ""` goes back to initials. Unknown avatars are rejected with `worker_validation`. |

Worker responses carry `avatar`, empty when the worker has none or its plugin is not enabled. Images need the same sign-in as the rest of the API; `?token=<access token>` works where a header cannot be set.
