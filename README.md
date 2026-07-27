# meshy

CLI for the [Meshy.AI](https://www.meshy.ai) REST API (`https://api.meshy.ai`).

Three layers:

1. **Resources** — thin 1:1 API surface (`text-to-3d`, `image-to-3d`, …)
2. **Profiles** — named default bundles (`--profile game-hero`)
3. **`ez` workflows** — multi-step jobs with sane naming, tags, and downloads

## Install / develop

```bash
nix build && ./result/bin/meshy --help
# or
nix develop
go run ./cmd/meshy --help
```

## Auth

Precedence: **flag → env → SQLite**.

```bash
export MESHY_API_KEY=msy_...
# or
meshy config set api_key msy_...
```

Default DB: `~/.config/meshy/meshy.db` (`--db` / `MESHY_DB`).

## Quick path: `ez`

```bash
# Text → preview → textured refine → download (glb by default)
meshy ez textured-3d \
  --prompt "beefcake minotaur" \
  --profile game-hero \
  --name minotaur \
  -o ./models/minotaur

# Image → textured mesh
meshy ez image-to-3d --image-url https://example.com/photo.png --profile game-lowpoly -o ./out/hero

# Hooks for scripting (repeatable --pre / --post)
meshy ez textured-3d --prompt "mug" -o ./out/mug \
  --pre 'mkdir -p ./out/mug/logs' \
  --post 'echo done task=$MESHY_TASK_ID glb=$MESHY_GLB' \
  --post 'cp {{glb}} ~/Dropbox/models/'

meshy ez profiles   # list built-in profiles
```

**Output:** `-o` / `--output` sets the directory (default `./out/<name>`; created if missing).
Files use a clean prefix (`minotaur-model.glb`); `manifest.json` records ids, profile, and paths.

**Hooks:** `--pre` runs before API work; `--post` runs after success with **cwd = output dir**.
Both use `sh -c`. Values are available as:

| Style | Examples |
|-------|----------|
| Env | `$MESHY_OUT_DIR`, `$MESHY_NAME`, `$MESHY_GLB`, `$MESHY_TASK_ID`, `$MESHY_MANIFEST`, `$MESHY_PREVIEW_ID`, `$MESHY_REFINE_ID`, `$MESHY_FILE_BASE_COLOR`, … |
| Placeholders | `{{out}}`, `{{name}}`, `{{glb}}`, `{{task_id}}`, `{{manifest}}`, `{{preview_id}}`, `{{refine_id}}`, `{{base_color}}`, … |

## Profiles

Global flag (applies to create / download / ez):

| Built-in | Intent |
|----------|--------|
| `game-lowpoly` | Remesh, ~30k polys, glb |
| `game-hero` | Medium poly, PBR, glb |
| `cinema` | Higher poly, glb+fbx + loose PBR maps |
| `print` | stl+glb bias |
| `fast` | Minimal defaults, glb |

```bash
meshy --profile cinema text-to-3d create-preview --prompt "…" --sync
```

**User profiles (CRUD, SQLite):**

```bash
meshy profile list
meshy profile show game-hero
meshy profile create my-game --from game-lowpoly --target-polycount 15000 --description "mobile"
meshy profile update my-game --download-formats glb,fbx --textures
meshy profile delete my-game
meshy --profile my-game ez textured-3d --prompt "…"
```

User profiles with the same name **shadow** built-ins. Deleting the user row restores the built-in. Built-ins cannot be deleted. Explicit flags always win over the profile.

## Local image files

Meshy accepts public URLs or **data URIs**. Local files are auto-encoded:

```bash
meshy image-to-3d create --image-file ./photo.png --sync --download
meshy image-to-3d create --image-url ./photo.png   # path also works
meshy multi-image-to-3d create --image-files a.png,b.png --sync
meshy ez image-to-3d --image ./photo.png -o ./out/hero
```

## Resources & async verbs

```text
create [--sync|--wait] [--download] [--format glb,fbx] [--textures]
get | list | delete | wait/sync | assets | download
```

```bash
meshy text-to-3d create-preview --prompt "a mug" --sync
meshy text-to-3d create-refine --preview-task-id <id> --enable-pbr --sync --download

meshy text-to-3d assets <id>
meshy text-to-3d download <id>                    # default: glb
meshy text-to-3d download <id> --format glb,fbx
meshy text-to-3d download <id> --format glb --textures
meshy text-to-3d download <id> --all -o ./out
```

Asset discovery covers `model_urls`, `image_urls`, nested `result.*`, and **`texture_urls` PBR maps** (`base_color`, `normal`, …).

## Image models / animation library

```bash
meshy text-to-image models
meshy animation library
```

## Sessions & tags

```bash
meshy session open my-batch
meshy --session my-batch --tag hero image-to-3d create --image-url … --sync
meshy tasks list --session my-batch
meshy tasks check --session my-batch
```

## Tests

```bash
go test ./...
```
