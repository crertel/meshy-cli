# meshy

CLI for the [Meshy.AI](https://www.meshy.ai) REST API (`https://api.meshy.ai`).

Suitable for humans, scripts, and agents: resource-oriented subcommands, human or `--json` output, local SQLite settings, and local work sessions/tags for batch job bookkeeping.

## Install / develop

```bash
# Nix
nix build
./result/bin/meshy --help

# or enter the dev shell
nix develop
go run ./cmd/meshy --help
```

## Auth

Config precedence: **flag → environment → SQLite**.

```bash
export MESHY_API_KEY=msy_...
# or
meshy config set api_key msy_...
# or
meshy --api-key msy_... balance get
```

Default SQLite path: `~/.config/meshy/meshy.db` (override with `--db` / `MESHY_DB`).

## Resource families

| Family | Commands |
|--------|----------|
| Generation | `text-to-3d`, `image-to-3d`, `multi-image-to-3d` |
| Post-process | `remesh`, `convert`, `resize`, `uv-unwrap`, `retexture`, `rigging`, `animation` |
| 2D | `text-to-image`, `image-to-image` |
| Print | `print multi-color`, `print analyze`, `print repair` |
| Creative Lab | `creative-lab <product> prototype|build` |
| Account | `balance` |

Shared async task verbs (where the API supports them):

```text
create [--sync|--wait] [--download] | get | list | delete | wait/sync | assets | download
```

Meshy tasks are asynchronous: **create** returns an id → **`--sync`** (alias `--wait`) polls until `SUCCEEDED` / `FAILED` / `CANCELED` → **assets** lists downloadable URLs → **download** saves them.

One-shot create + wait + download:

```bash
meshy image-to-3d create --image-url https://... --sync --download -o ./out
```

Text-to-3D uses the two-step API:

```bash
meshy text-to-3d create-preview --prompt "a coffee mug" --sync
meshy text-to-3d create-refine --preview-task-id <id> --sync --download
```

### Discover models and assets

```bash
# Known ai_model values for image endpoints (from docs)
meshy text-to-image models
meshy image-to-image models

# Public animation action_id catalog
meshy animation library

# What's downloadable on a finished task?
meshy text-to-image assets <task-id>
meshy image-to-3d assets <task-id>
meshy text-to-image download <task-id> --all -o ./out
meshy image-to-3d download <task-id> --name glb
```

## Sessions and tags

Local only (not Meshy cloud sessions). Track batches of jobs you create:

```bash
meshy session open my-batch
meshy --session my-batch --tag hero image-to-3d create --image-url https://...
meshy tasks list --session my-batch
meshy tasks check --session my-batch   # refresh remote status
meshy session show my-batch
```

## JSON output

```bash
meshy --json balance get
meshy --json image-to-3d get <task-id>
```

## Examples

```bash
# Image to 3D (sync run)
meshy image-to-3d create --image-url https://example.com/photo.png --should-texture --sync --download -o ./out

# Text to image
meshy text-to-image create --ai-model nano-banana --prompt "a dragon at sunset" --sync
meshy text-to-image assets <task-id>
meshy text-to-image download <task-id> --kind image -o ./imgs

# Raw JSON body when flags are not enough
meshy remesh create --body '{"input_task_id":"...","target_formats":["glb"]}' --sync

# Credits
meshy balance get
```

## Tests

```bash
go test ./...
```

Tests drive the real HTTP client and CLI against a mock Meshy server (create → poll → terminal) and exercise SQLite settings/session/tag storage.
