# ollama-mgr

Thin Windows manager for **local Ollama models**: list, check for updates (same-tag digests **and** notional generation upgrades), delete, open library pages, pull, and run.

Ollama does not really “manage” what you have downloaded. This fills that gap.

## Features

| Capability | Description |
|------------|-------------|
| **List** | Installed models with size, params, quant, **Released** (upstream library Updated), Downloaded (local), library URL |
| **Family view** | Group by base model with **feature pills** (tools/vision/…) and **size pills** (solid = downloaded, outline = available → click to pull) |
| **Batch (GUI)** | Checkboxes + select-all for batch check / open / delete |
| **Digest updates** | Compare local weight digests to `registry.ollama.ai` without pulling |
| **Notional upgrades** | e.g. `qwen2.5-coder:32b` → `qwen3-coder:30b` (same weight class + specialty, newer series) |
| **Upgrade modes** | Skip · side-by-side · **staged swap** (tag old as DELETE PENDING, show pull progress row, verify, only then delete) |
| **Delete / pull / run** | Thin wrappers over the Ollama API + CLI |
| **Open listing** | Browser link to `https://ollama.com/library/...` |
| **CLI + GUI** | Shared core; two Windows executables |

## Build (Windows)

Requirements: [Go](https://go.dev/) 1.22+, and [Ollama](https://ollama.com) for live use.

```powershell
go mod tidy
.\scripts\build.ps1
```

Produces:

- `dist/ollama-mgr.exe` — CLI
- `dist/ollama-mgr-gui.exe` — WebView2 GUI (no console window; needs WebView2 Runtime, preinstalled on modern Windows 11)

## CLI

```text
ollama-mgr list
ollama-mgr list --family
ollama-mgr list --sort size --desc
ollama-mgr list --sort params
ollama-mgr list --sort released
ollama-mgr check
ollama-mgr check --json
ollama-mgr upgrade qwen2.5-coder:32b --to qwen3-coder:30b --mode side-by-side
ollama-mgr upgrade llama3.1:8b --mode pull
ollama-mgr upgrade old:tag --to new:tag --mode swap --yes
ollama-mgr rm mymodel:tag --yes
ollama-mgr pull mistral:7b
ollama-mgr open qwen2.5-coder:32b
ollama-mgr run llama3.1:8b
ollama-mgr status
ollama-mgr serve
ollama-mgr --endpoint http://gpu-box:11434 list
```

Environment:

| Variable | Meaning |
|----------|---------|
| `OLLAMA_HOST` | Ollama API host (default `http://localhost:11434`) |
| `OLLAMA_MGR_CACHE_TTL_HOURS` | Catalog cache TTL (default 24) |

## GUI

Double-click `ollama-mgr-gui.exe`, or run it from a terminal. Use the toolbar to refresh, check updates, upgrade (skip / side-by-side / swap), open the library page, run, or delete.

## How notional upgrades work

From a name like `qwen2.5-coder:32b` we parse:

- **Family** `qwen`
- **Version** `2.5`
- **Specialty** `coder`
- **Size** `32b` (or from `parameter_size` when the tag is `latest`)

Then we search the Ollama library for the same family + specialty with a **newer** series version and a **compatible** size tag, and rank candidates. Nothing is pulled until you choose a mode.

Same-tag **digest** updates are reported separately when upstream weights changed for the exact tag you already have.

## Layout

```text
cmd/ollama-mgr/       CLI
cmd/ollama-mgr-gui/   GUI entrypoint
internal/ollama/      Local Ollama HTTP API
internal/registry/    registry.ollama.ai manifests
internal/catalog/     ollama.com search/tags (cached)
internal/modelparse/  Name → family/version/specialty/size
internal/upgrade/     Digest + notional engine
internal/actions/     skip / side-by-side / swap / pull
internal/ui/gui/      WebView2 + embedded HTML UI
```

## GUI notes

The GUI binary embeds a small local HTTP API and hosts a WebView2 window (falls back to your default browser if WebView2 is unavailable). No C compiler / CGO required to build.


## License

MIT (or as you prefer — add a LICENSE when publishing).
