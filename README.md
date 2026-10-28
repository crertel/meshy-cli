# meshy

CLI for the [Meshy](https://www.meshy.ai) REST API (`https://api.meshy.ai`).

Three layers:

1. **Resources** — thin 1:1 API surface (`text-to-3d`, `image-to-3d`, `retexture`, …)
2. **Profiles** — named default bundles (`--profile game-hero`)
3. **`ez` workflows** — multi-step jobs with sane naming, tags, and downloads

## Install

Requires [Nix](https://nixos.org/download/) with flakes (`experimental-features = nix-command flakes`).

### From a blank machine (GitHub flake)

```bash
# One-shot
nix run github:crertel/meshy-cli -- --help
nix run github:crertel/meshy-cli -- version

# Temporary shell with meshy on PATH
nix shell github:crertel/meshy-cli

# Install into your user profile
nix profile install github:crertel/meshy-cli
meshy --help
```

Arguments after `--` go to the program:

```bash
nix run github:crertel/meshy-cli -- ez textured-3d --prompt "a mug" -o ./out/mug
```

### From a local checkout

```bash
git clone https://github.com/crertel/meshy-cli
cd meshy-cli

nix build                 # → ./result/bin/meshy
nix run                   # run default app
nix profile install .     # install this checkout
nix flake check           # build package as a check
nix develop               # dev shell (built meshy + Go tooling)
```

Layout:

| File | Role |
|------|------|
| `package.nix` | Derivation (`buildGoModule`); usable via `callPackage` |
| `flake.nix` | Overlay, packages, apps, checks, devShell |

### Consume from another flake

```nix
{
  inputs.meshy-cli.url = "github:crertel/meshy-cli";

  # Option A: packages
  # packages = [ meshy-cli.packages.${system}.meshy ];

  # Option B: overlay → pkgs.meshy
  # nixpkgs.overlays = [ meshy-cli.overlays.default ];
}
```

### Without Nix

```bash
go install github.com/crertel/meshy-cli/cmd/meshy@latest
# or from a checkout:
go build -o meshy ./cmd/meshy
```

## Auth

Precedence: **flag → env → SQLite config**.

```bash
export MESHY_API_KEY=msy_...
# or
meshy config set api_key msy_...
meshy balance get
```

Default DB: `~/.config/meshy/meshy.db` (`--db` / `MESHY_DB`).

## Quick path: `ez`

```bash
# Text → preview → textured refine → download
meshy ez textured-3d \
  --prompt "beefcake minotaur" \
  --profile game-hero \
  --name minotaur \
  -o ./models/minotaur

# Image → textured mesh (local file or URL)
meshy ez image-to-3d --image ./photo.png --profile game-lowpoly -o ./out/hero

# Multi-view images
meshy ez multi-image-to-3d --images front.png,side.png,back.png -o ./out/multi

# Image-to-image (2D transform)
meshy ez image-to-image --image ref.png --prompt "cyberpunk style" -o ./out/i2i

# Post-process existing Meshy tasks
meshy ez retexture --input-task-id <id> --texture-prompt "gold armor" -o ./out/rt
meshy ez remesh --input-task-id <id> --target-polycount 30000 -o ./out/rm

# Print helpers
meshy ez print-analyze --input-task-id <id>
meshy ez print-repair --input-task-id <id> -o ./out/print

# Hooks (repeatable). Trailing & detaches GUI apps so the CLI can exit.
meshy ez image-to-3d --image ./photo.png -o ./out/hero \
  --post 'xviewer {{glb}} &'

meshy ez profiles   # list built-in + user profiles
```

**Output:** `-o` / `--output` sets the directory (default `./out/<name>`; created if missing).
Files use a clean prefix (`minotaur-model.glb`); `manifest.json` records ids, profile, and paths.

**Hooks:** `--pre` runs before API work; `--post` after success with **cwd = output dir**.
Both use `sh -c`.

| Style | Examples |
|-------|----------|
| Env | `$MESHY_OUT_DIR`, `$MESHY_NAME`, `$MESHY_GLB`, `$MESHY_TASK_ID`, `$MESHY_MANIFEST`, `$MESHY_PREVIEW_ID`, `$MESHY_REFINE_ID`, `$MESHY_FILE_BASE_COLOR`, … |
| Placeholders | `{{out}}`, `{{name}}`, `{{glb}}`, `{{task_id}}`, `{{manifest}}`, `{{preview_id}}`, `{{refine_id}}`, `{{base_color}}`, … |

Create and resource commands also support `--pre` / `--post`.

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

User profiles with the same name **shadow** built-ins. Deleting the user row restores the built-in.
Built-ins cannot be deleted. Explicit flags always win over the profile.

## Create flags

Every resource `create` accepts convenience flags mapped to Meshy JSON fields
(plus `--body` / `--body-file` for anything else). Flags only send values when set.

Common examples:

```bash
meshy image-to-3d create \
  --image-file ./photo.png \
  --ai-model latest \
  --should-texture --enable-pbr \
  --should-remesh --target-polycount 50000 \
  --pose-mode a-pose \
  --target-formats glb,fbx \
  --texture-resolution 4k \
  --auto-size --origin-at bottom \
  --sync --download -o ./out

meshy retexture create \
  --input-task-id <id> \
  --text-style-prompt "gold armor" \
  --enable-original-uv --enable-pbr \
  --sync --download

meshy animation create \
  --rig-task-id <id> \
  --action-id 92 \
  --post-process-operation change_fps \
  --post-process-fps 24 \
  --sync

meshy print multi-color create --input-task-id <id> --max-colors 8 --style realistic --printer-brand bambu --sync --download

meshy print split create --input-task-id <id> --mode by_parts --prompt "head, torso" --connectors --layout on_plate

meshy text-to-motion create --prompt "a character waving" --duration 3 --mode swift --sync

meshy rigging create --input-task-id <id> --animation-type quadruped --height-meters 0.4

meshy animation create --rig-task-id <id> --action-ids 10,25,92
meshy animation create --rig-task-id <id> --motion-task-id <text-to-motion-id>

meshy image-to-3d create --image-file ./a.png --geometry-resolution 2k --ai-model meshy-7.1
meshy image-to-3d create --image-file ./a.png --model-type smart-topology --ai-model meshy-t2.5 --target-polycount 8000

meshy creative-lab keycap build create --input-task-id <id> --candidate-id <candidate>
meshy creative-lab fidget-pixel build create --input-task-id <id> --shape hex --grid-size 16 --output-format 3mf
meshy creative-lab fidget-collapsible create --image-file ./photo.png --sync --download

meshy --metadata user=alex@example.com --metadata team=fab image-to-3d create --image-file ./a.png
```

`latest` on text-to-3d, image-to-3d, and multi-image-to-3d is `meshy-7.1`. On retexture, `latest` is `meshy-7` (`meshy-7.1` is rejected there). `meshy-5` is retired. `model_type: lowpoly` retires on 2026-10-30; use `smart-topology` with `meshy-t2.5` or `meshy-t2`.

`--name` is the download filename prefix. The API task name is `--task-name`.

Deprecated Meshy fields (`ultra_mode`, `hd_texture`, `symmetry_mode`, `art_style`, `is_a_t_pose`) are **not** exposed as flags; use `--body` if you need them. `geometry_resolution` replaces `ultra_mode`. Rigging `texture_image_url` is deprecated: omit it so the rig keeps the model's textures.

Local paths work for images and models where the API accepts a URL or data URI
(`--image-url`, `--image-file`, `--image-urls`, `--model-url`, …).

## Resources & async verbs

```text
create [--sync|--wait] [--download] [--stream] [--format glb,fbx] [--textures]
get | list | delete | wait [--stream] | stream | assets | download
```

```bash
meshy text-to-3d create-preview --prompt "a mug" --sync
meshy text-to-3d create-refine --preview-task-id <id> --enable-pbr --sync --download

# Wait: poll (default) or SSE stream
meshy image-to-3d wait <id>
meshy image-to-3d wait <id> --stream
meshy image-to-3d stream <id>
meshy --stream image-to-3d create --image-file ./a.png --sync   # prefer SSE globally

meshy text-to-3d assets <id>
meshy text-to-3d download <id>                    # default: glb
meshy text-to-3d download <id> --format glb,fbx
meshy text-to-3d download <id> --format glb --textures
meshy text-to-3d download <id> --all -o ./out
```

Resource families:

| Group | Commands |
|-------|----------|
| generation | `text-to-3d`, `image-to-3d`, `multi-image-to-3d` |
| postprocess | `remesh`, `convert`, `resize`, `uv-unwrap`, `retexture`, `rigging`, `animation`, `text-to-motion` |
| image | `text-to-image`, `image-to-image` |
| print | `print multi-color`, `print analyze`, `print repair`, `print split` |
| creative-lab | `creative-lab <product> prototype\|build`; `fidget-collapsible` is a single stage |
| account | `balance`, `usage list` |

Creative Lab products: `keychain`, `fridge-magnet`, `figure`, `vinyl-figure`, `brick-figure`, `lamp`, `keycap`, `fidget-pixel`, `fidget-collapsible`.

Notes:

- Rigging and animation both support `list`.
- `list --sort` sends `sort_by` (default on the API is `-created_at`).
- Asset discovery covers `model_urls`, `image_urls`, nested `result.*_url` (including animation and text-to-motion clips), and **`texture_urls`** PBR maps (`base_color`, `normal`, …).

## Image models / animation library

```bash
meshy text-to-image models
meshy animation library
meshy animation library --category Fighting --search walk
meshy usage list --start 2026-08-01T00:00:00Z --endpoint image-to-3d,text-to-3d-refine
```

Image `ai_model` values: `nano-banana`, `nano-banana-2`, `nano-banana-2-lite`, `nano-banana-pro`, `gpt-image-2`, `gpt-image-2-5-flare`, `gpt-image-2-5-sunburst`.

The animation catalog is `GET /openapi/v1/animations/library` and requires an API key.

## Sessions & tags

```bash
meshy session open my-batch
meshy --session my-batch --tag hero image-to-3d create --image-file ./a.png --sync
meshy tasks list --session my-batch
meshy tasks check --session my-batch
```

## Global flags

| Flag | Purpose |
|------|---------|
| `--api-key` | Meshy API key |
| `--base-url` | API base (default `https://api.meshy.ai`) |
| `--db` | SQLite path |
| `--profile` | Built-in or user profile |
| `--session` / `--tag` | Local tracking |
| `--metadata` | `X-Meshy-Metadata` `key=value` on create (Studio/Enterprise; ignored by Creative Lab) |
| `--stream` | Prefer SSE wait over polling |
| `--poll-interval` / `--wait-timeout` | Wait tuning |
| `--json` / `--quiet` | Output control |

## Develop / test

```bash
nix develop
go test ./...
go build -o meshy ./cmd/meshy
```

API reference: [docs.meshy.ai](https://docs.meshy.ai).
