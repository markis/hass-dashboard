# AGENTS.md

Essential context for AI coding agents working on home-dashboard.

## Overview

Go dashboard-image generator for e-ink displays: fetches Google Calendar events
(service account with Workspace domain-wide delegation) plus OpenWeatherMap
weather (One Call API), renders an HTML template, and screenshots it with
chromedp (PNG → JPEG) to the configured output path at a fixed interval. Single
binary, entry point `cmd/home-dashboard/main.go`.

Module `github.com/markis/home-dashboard`, Go from `go.mod` (CI reads the
version from go.mod).

## Commands

```bash
go build -o home-dashboard ./cmd/home-dashboard
go test ./...                          # hermetic: no Chrome, network, or credentials needed
go test ./internal/clients -run TestWeatherClientGetWeather
golangci-lint run                      # lint AND format checks (see .golangci.yml formatters)
golangci-lint fmt                      # auto-apply gofumpt + goimports with module local-prefix
./home-dashboard --config config.yaml  # run (only after go build)
```

- Format with `golangci-lint fmt`, **not** plain `gofumpt -w`: the gofumpt
  `extra` rules (`clothe-returns`, `group-params`) and the goimports
  `local-prefixes` live in `.golangci.yml`, and `golangci-lint run` fails on
  formatting violations.
- CI runs `golangci-lint` pinned to v2.15.0 (lint.yml) so fix locally with the
  same version.
- CI tests with `go test -race` and `CGO_ENABLED=1` (race detector requirement;
  needs a C toolchain). Plain `go test ./...` is fine for fast local iteration.
- `.golangci.yml` uses an explicit linter enable list (not `default: all`) so
  renamed/removed linters can't silently start failing CI — add new linters
  there on purpose. Hard limits (funlen 100 lines/50 statements, lll 120,
  gocognit 20, gocyclo 15, varnamelen) are configured in that file; most are
  exempted for `*_test.go`.
- depguard denies `io/ioutil` and `github.com/pkg/errors` — use `os`/`io` and
  stdlib `errors`; wrap errors with `fmt.Errorf("...: %w", err)`.

## Project conventions (differ from Go defaults)

- Logging is `log/slog` with a JSON handler installed as the *default* logger in
  `main` — just call `slog.Info/Error/...` anywhere. Messages are plain
  human-readable phrases; values go in key/value attributes with snake_case keys
  (`"calendar_id"`, `"error"`), never fmt.Sprintf-ed into the message. The JSON
  handler serializes `time.Duration` as raw nanoseconds — log durations as
  `.String()` strings when humans will read them.
- Fatal startup failures go through the `fail` helper in `main.go` (log + exit 1
  + optional cleanup), not `panic` or bare `os.Exit`.
- Config is YAML (`gopkg.in/yaml.v3`) with tags matching keys exactly
  (`yaml:"credentials_file"`). `config.yaml` and `credentials.json` are
  gitignored — copy `config.example.yaml` for local runs.
- Tests are table-driven with `t.Run` subtests and hit `httptest.NewServer()`
  against client endpoints; no test spawns Chrome or the network.

## Gotchas

- Template assets are compiled in via `go:embed` from
  `internal/render/templates/` (`dashboard.html`, `style.css`,
  `weather-icons-embedded.css`). Editing them requires a rebuild — nothing is
  read from disk at runtime.
- Weather Icons fonts are codegen'd and gitignored (`static/fonts/*.woff*`,
  `*.css`): the Docker build runs `scripts/download-weather-icons.sh` +
  `scripts/fonts-to-css.sh`, which write to `static/fonts/`. The binary embeds
  the committed `internal/render/templates/weather-icons-embedded.css`, so
  changing rendered icons means copying fresh CSS into that template file;
  `static/fonts/weather-icons-embedded.css` copied into the runtime image is
  vestigial.
- Actually rendering an image needs a real Chrome/Chromium binary for chromedp
  (containers set `CHROME_PATH=/usr/bin/chromium-browser`); pure-Go tests don't.
- Renders are skipped when the input-data SHA-256 matches the stored
  `output.path + ".hash"` — on unchanged data the image file is intentionally
  left untouched (the serve endpoint then answers 304; the hash file is touched
  instead so the healthcheck still sees freshness).

## Architecture

```
main.go generation loop (per refresh_interval):
  fetch (weather + calendar concurrently) → buildTemplate → render.Image
```

- `internal/clients` — Google Calendar: service-account JWT self-signs tokens
  (`google.JWTConfigFromJSON`, `calendar.CalendarReadonlyScope`, Workspace user
  impersonation via `jwtConfig.Subject`; no `token.json`). Paginated
  `Events.List(...).SingleEvents(true)` calls; pure `groupEventsByDate` helper
  groups by date. Weather: OpenWeatherMap One Call API, 30s HTTP timeout,
  `sync.RWMutex` cache keyed by lat/long with 10-minute TTL and stale-cache
  fallback after fetch errors.
- `internal/render` — html/template assembly (pure, fully unit-testable) plus
  the only chunk requiring Chrome: chromedp FullScreenshot → JPEG, written
  atomically via temp file + rename.
- `internal/serve` — read-only HTTP image server; requests never trigger a
  render (404 until the first image exists on disk).
- `internal/telemetry` — OTLP traces/metrics, off by default (`otel.enabled`).
  `telemetry.Init` must run before building instrumented components.
- `internal/models` — shared data types.

## CI / release flow

- Pushes to `main` auto-release a Docker image to ghcr.io
  (`.github/workflows/docker.yml`): the patch version is auto-bumped from the
  newest `v*` tag and the tag is pushed automatically. Don't hand-create patch
  tags for main merges; manual `v*` tags are respected as-is.
- Tests: push/PR to `main` runs the race-enabled suite plus a per-function
  coverage summary; lint pins the golangci-lint version.
- Dependabot: Go minor+patch bumps are grouped into one PR with the
  `chore(deps)` prefix; major bumps get separate PRs.