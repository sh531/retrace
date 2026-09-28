# retrace
Go tool that locates hike photos along a GPX track using EXIF GPS and timestamp interpolation, then tags them with nearby OpenStreetMap landmarks. Generates a self-contained HTML map with a photo flythrough for revisiting or sharing viewpoints.

## Development

### Dev container (recommended)

The repo includes a dev container with Go 1.27.1 and the Git hooks pre-configured.

- **VS Code / Cursor:** install the Dev Containers extension (in Cursor: `@id:anysphere.remote-containers`),
  then run **Dev Containers: Reopen in Container**. Requires Docker.
- **Terminal only** (requires Docker and Node):

  ```sh
  npx --yes @devcontainers/cli up --workspace-folder .
  npx --yes @devcontainers/cli exec --workspace-folder . bash
  ```

### Local

Requires Go 1.27.1+ and golangci-lint (version in `.golangci-lint-version` [install docs](https://golangci-lint.run/docs/welcome/install/local/)).
Point Git at the shared hooks so tidy, gofmt, vet, and lint checks run before each commit:

```sh
git config core.hooksPath .githooks
```

### Build and test

```sh
make check
```
