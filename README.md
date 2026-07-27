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
  --name minotaur

# Image → textured mesh
meshy ez image-to-3d --image-url https://example.com/photo.png --profile game-lowpoly

meshy ez profiles   # list built-in profiles
```

Outputs land in `./out/<name>/` with `model.glb` (or profile formats), optional textures, and `manifest.json` (task ids, seed, paths).

## Profiles

Global flag (applies to create / download / ez):

| Profile | Intent |
|---------|--------|
| `game-lowpoly` | Remesh, ~30k polys, glb |
| `game-hero` | Medium poly, PBR, glb |
| `cinema` | Higher poly, glb+fbx + loose PBR maps |
| `print` | stl+glb bias |
| `fast` | Minimal defaults, glb |

```bash
meshy --profile cinema text-to-3d create-preview --prompt "…" --sync
```

Explicit flags always win over the profile.

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
