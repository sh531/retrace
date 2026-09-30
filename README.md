# retrace

Go tool that locates hike photos along a GPX track using EXIF GPS and timestamp interpolation, then tags them with nearby OpenStreetMap landmarks. Generates a self-contained HTML map with a photo flythrough for revisiting or sharing viewpoints.

## Install

Requires Go 1.27.1+.

```sh
go install github.com/sh531/retrace/cmd/retrace@latest
```

Or from a local clone:

```sh
git clone https://github.com/sh531/retrace.git
cd retrace
go install ./cmd/retrace
```

Both install `retrace` to `$(go env GOPATH)/bin` (usually `~/go/bin`), which needs to be on your `PATH`.

## Usage

> The CLI is still being built; commands and flags may change.

retrace takes a folder of photos and a GPX track (with timestamps) recorded on the same hike.

### Finding your camera's offset

If the photos have no GPS, retrace places them on the track by time. `--offset` is how much to add to your camera's clock to get UTC.

1. With the camera, take a photo of your phone's clock.
2. Read the camera's time from that photo: `exiftool -DateTimeOriginal photo.jpg`.
3. Convert the time shown on the phone to UTC, then subtract the camera's time:
  ```
   offset = phone time (UTC) − camera time
  ```
   For example, the phone shows 08:00:00 PDT, which is 15:00:00 UTC, and the camera recorded 08:07:30:
   If the camera's time is later than UTC, the offset is negative, e.g. `--offset -5h30m`.

The same offset works for every photo until you change or reset the camera's clock.

## Design decisions

- **Camera time is a fixed offset from true time.** EXIF `DateTimeOriginal` has no timezone, and a camera clock that is never adjusted doesn't follow travel or daylight savings. retrace reads it as UTC and applies a single `--offset`, the duration to add to camera time to get UTC, which covers both the timezone difference and clock drift. See [Finding your camera's offset](#finding-your-cameras-offset).
- **A photo's location is optional and all-or-nothing.** `Photo.Location` is a `*Location` holding the point and how it was determined (`exif` or `interpolated`), and is `nil` when the photo couldn't be located. Grouping them means a point can't exist without a source or vice versa. `nil` keeps the zero value (0,0), a real place in the Atlantic, from being plotted by mistake.
- **Track points need a time; elevation is optional.** Time is what places a photo on the track, so a point without `<time>` fails the GPX file parsing step. The usual cause is exporting a trail's planned route instead of a recorded activity. Elevation is `*float64` and `nil` when `<ele>` is missing, because 0 m is sea level. A zero default would draw a fake drop to sea level in the elevation profile. NaN isn't used because `encoding/json` can't encode it.



## Project structure

The layout follows the Go team's [Organizing a Go module](https://go.dev/doc/modules/layout) guide. The module contains a [command with supporting packages](https://go.dev/doc/modules/layout#package-or-command-with-supporting-packages), with the command in `cmd/retrace/` and its supporting packages in `internal/`.

```
retrace/
├── cmd/
│   └── retrace/
│       └── main.go              # CLI entrypoint (package main)
├── internal/                    # supporting packages, importable only within this module
├── .devcontainer/
│   ├── devcontainer.json        # dev container: Go, Git hooks, editor extensions
│   ├── devcontainer-lock.json   # pinned dev container feature versions
│   ├── Dockerfile               # base image + exiftool + golangci-lint
│   └── Dockerfile.dockerignore  # limits the build context to files the Dockerfile copies
├── .githooks/
│   └── pre-commit               # tidy, gofmt, vet, and lint before each commit
├── .github/
│   └── workflows/
│       └── ci.yml               # tidy, gofmt, vet, test -race, build, and lint on push/PR
├── .gitignore
├── .golangci-lint-version       # pinned golangci-lint version (Makefile, CI, dev container)
├── .golangci.yml                # linter configuration
├── go.mod
├── LICENSE
├── Makefile                     # check, build, test, and lint targets
└── README.md
```



## Development



### Dev container (recommended)

The repo includes a dev container with Go 1.27.1 and the Git hooks pre-configured.

- **VS Code / Cursor:** install the Dev Containers extension (in Cursor: `@id:anysphere.remote-containers`), then run **Dev Containers: Reopen in Container**. Requires Docker.
- **Terminal only** (requires Docker and Node):
  ```sh
  npx --yes @devcontainers/cli up --workspace-folder .
  npx --yes @devcontainers/cli exec --workspace-folder . bash
  ```



### Local

Requires Go 1.27.1+ and golangci-lint (version in `.golangci-lint-version`, [install docs](https://golangci-lint.run/docs/welcome/install/local/)). Point Git at the shared hooks so tidy, gofmt, vet, and lint checks run before each commit:

```sh
git config core.hooksPath .githooks
```



### Build and test

```sh
make check              # everything CI runs
make build              # build to bin/retrace
go run ./cmd/retrace    # run without installing
```

