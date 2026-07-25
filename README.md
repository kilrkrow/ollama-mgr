# ollama-mgr

Thin Windows manager for **local Ollama models**: list, check for updates (same-tag digests **and** notional generation upgrades), delete, open library pages, pull, and run.

Ollama does not really “manage” what you have downloaded. This fills that gap.

**Repo:** https://github.com/kilrkrow/ollama-mgr

## Features

| Capability | Description |
|------------|-------------|
| **List** | Installed models with size, params, quant, **Released** (upstream library Updated), Downloaded (local), library URL |
| **Family view** | Group by base model with **feature pills** and **size pills** (solid = on disk, outline = available → click to pull) |
| **+ Family** | Search the library and fetch a line you don’t have yet (features + outline sizes; nothing downloads until you click a pill). Board persists under `%APPDATA%\ollama-mgr\` |
| **Popular** | Browse top 10/25/50/100 by ollama.com library order (download rank). Features explain “why care”; Fetch / size pills pull explicitly |
| **Flags** | Curated country-of-origin chips (lab HQ). **Not** from the Ollama API — see `internal/origin` |
| **Selection** | Click row = select · Ctrl+click = toggle · Shift+click = range · Esc = clear (no always-on checkboxes) |
| **Digest updates** | Compare local weight digests to `registry.ollama.ai` without pulling |
| **Notional upgrades** | e.g. `qwen2.5-coder:32b` → `qwen3-coder:30b` (same weight class + specialty, newer series) |
| **Upgrade modes** | Skip · side-by-side · **staged swap** (DELETE PENDING → pull/verify → only then remove old) |
| **Run model** | Opens a new console (`ollama run <tag>`). Needs exactly one selected installed model |
| **Start server** | Starts the Ollama daemon if down (`ollama serve`). No model required |
| **CLI + GUI** | Shared core; two Windows executables |

### Feature chips (legend)

| Style | Meaning |
|--------|---------|
| Solid indigo | Reported on **your installed** tag(s) |
| Dashed / muted | Advertised on the **library** page for the family, not confirmed on local tags |

### Size pills (legend)

| Style | Meaning |
|--------|---------|
| Solid green | That size class is **downloaded** |
| Dashed outline | On the library, **not** local — click to pull |

## Build (Windows)

Requirements: [Go](https://go.dev/) 1.22+, and [Ollama](https://ollama.com) for live use.

```powershell
go mod tidy
.\scripts\build.ps1
```

Produces:

- `dist/ollama-mgr.exe` — CLI
- `dist/ollama-mgr-gui.exe` — WebView2 GUI (no console flash; needs WebView2 Runtime, preinstalled on modern Windows 11)

```powershell
go install github.com/kilrkrow/ollama-mgr/cmd/ollama-mgr@latest
```

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
ollama-mgr version
ollama-mgr --endpoint http://gpu-box:11434 list
```

Environment:

| Variable | Meaning |
|----------|---------|
| `OLLAMA_HOST` | Ollama API host (default `http://localhost:11434`) |
| `OLLAMA_MGR_CACHE_TTL_HOURS` | Catalog cache TTL (default 24) |
| `OLLAMA_MGR_CONFIG_DIR` | Override config dir (default `%APPDATA%\ollama-mgr`) |

## GUI

Double-click `ollama-mgr-gui.exe`, or run it from a terminal.

- **Family \| Tag** — grouped catalog vs one row per installed tag  
- **+ Family** — fetch a library line (e.g. type `mistral`)  
- Select a model (row or solid size pill) → **Run model**  
- **Start server** only if the daemon is down  

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
internal/family/      Group local + fetched families, pills
internal/origin/      Curated country-of-origin map
internal/upgrade/     Digest + notional engine
internal/actions/     skip / side-by-side / swap / pull
internal/jobs/        Async upgrade jobs for GUI
internal/ui/gui/      WebView2 + embedded HTML UI
```

## Screenshots

Capture from a real install and drop into `docs/screenshots/` (optional):

1. **Family** — Ctry column, solid/outline sizes, feature chips  
2. **Tag** — status column shows ASCII `-` / `OK` / update text (no mojibake)  
3. **Popular** — top 10/25 with features and Fetch  
4. **+ Family** — search hits and fetched empty matrix  

Until then, run the GUI against your local Ollama to verify.

## License

MIT — see [LICENSE](LICENSE).
