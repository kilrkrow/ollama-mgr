# Releasing ollama-mgr

Releases are cut by pushing a tag. Everything else is done by
`.github/workflows/release.yml` on `windows-latest`.

| Tag            | GitHub release           | Chocolatey                  |
|----------------|--------------------------|-----------------------------|
| `v0.1.0`       | published, with assets   | pushed if allowed (below)   |
| `v0.1.0-rc.1`  | DRAFT prerelease         | packed + tested, never pushed |

Release assets:

- `ollama-mgr-win-x64-v<version>.zip` - `ollama-mgr.exe` (CLI),
  `ollama-mgr-gui.exe` (GUI), `LICENSE`, `README.md`
- `ollama-mgr-win-x64-v<version>.zip.sha256` - `<sha256>  <zip name>`

## Cut a release

1. Make sure `main` is green and `internal/config/config.go` has the version
   you want as its default (the build stamps the tag version in any case).
2. `git tag v0.1.0 && git push origin v0.1.0`
3. Watch the `release` workflow. For a prerelease tag, review the draft and
   publish (or delete) it by hand.

## Build the same zip locally

```powershell
.\build\release.ps1                  # version from internal/config/config.go
.\build\release.ps1 -Version 0.1.0   # explicit
```

Output goes to `release\` (self-ignored, never committed). The build is
reproducible: `-trimpath`, `-buildvcs=false`, CGO off, pinned `go-winres`,
zip written by `build/mkzip` (Go) with fixed timestamps and entry order. The Go toolchain comes from the `go`
line in `go.mod` (CI uses it via `setup-go` `go-version-file`).

## Chocolatey

Package id: `ollama-mgr` (source in `choco/`, filled in by
`build/choco-pack.ps1`). The package downloads the release zip from GitHub
and checks its SHA256; no binaries are embedded.

The push step runs only when all of these hold, otherwise it logs
`Chocolatey push SKIPPED: <reasons>` and exits 0:

- the tag has no `-` (not a prerelease)
- the repository is `kilrkrow/ollama-mgr` (not a fork)
- the repository is public
- the `CHOCOLATEY_API_KEY` secret is set

Every run also test-installs a copy of the package that points at the local
zip (`file:///`), checks files, shims and `ollama-mgr version`, then
uninstalls and checks that the lib folder, shims, the `KilrKrowOllamaMgr`
Run value and the `ollama-mgr.lnk` Start Menu shortcut are gone. That test
package is never pushed.

The first push of a new id goes through Chocolatey community moderation.
