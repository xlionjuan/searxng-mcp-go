# Release Workflow

Run this workflow only when the user explicitly asks for a release and the
target version is known or confirmed.

## Version Rules

- Tag style: `v{major}.{minor}.{patch}`, for example `v1.0.4` or `v1.1.0`.
- Do not use `-beta`, `-rc`, or other suffixes.
- Do not infer the target version. If it is missing or ambiguous, ask before
  editing release files.

## Steps

1. Patch the `version` constant in `main.go` to the new version.
2. Commit with a non-interactive message:

   ```bash
   git commit -m "chore: bump version to vX.Y.Z"
   ```

3. Create an annotated tag:

   ```bash
   git tag -a vX.Y.Z -m "Release vX.Y.Z"
   ```

4. Push main and the tag:

   ```bash
   git push origin main
   git push origin vX.Y.Z
   ```

Pushing the tag triggers `.github/workflows/release.yml`.

## GoReleaser Outputs

The `.goreleaser.yaml` configuration produces:

- `tar.zst` archives for `linux/amd64` and `linux/arm64`
- Static binaries with `CGO_ENABLED=0`
- Archive names matching `searxng-mcp-go_v{Version}_{Os}_{Arch}`
- Binary, `README.md`, `LICENSE`, and checksums file in release artifacts
- GitHub Release with automatic prerelease detection and `make_latest: true`
- Homebrew tap update for `xlionjuan/homebrew-tap`
- Auto-generated changelog excluding `docs:`, `test:`, and `chore:` commits

## Version Injection

GoReleaser ldflags override the hardcoded `main.go` version at build time:

```text
-X main.version=v{{.Version}} -X main.commit={{.Commit}} -X main.date={{.Date}}
```

The hardcoded `main.go` value is used by `--version` only when the binary is not
built by GoReleaser.
