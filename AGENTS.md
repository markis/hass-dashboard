# AGENTS.md

This document contains essential information for AI coding agents working in the hass-dashboard repository.

## Project Overview

A Go-based dashboard image generator for Home Assistant that displays calendar events and weather forecasts. Uses chromedp for rendering HTML to PNG images suitable for e-ink displays.

**Module**: `github.com/markis/hass-dashboard`
**Go Version**: 1.25.0

## Build, Lint, and Test Commands

### Build
```bash
# Build the main binary
go build -o hass-dashboard ./cmd/hass-dashboard

# Build with verbose output
go build -v -o hass-dashboard ./cmd/hass-dashboard

# Install to $GOPATH/bin
go install ./cmd/hass-dashboard
```

### Test
```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run tests with coverage
go test -cover ./...
go test -coverprofile=coverage.out ./...

# Run a single test file
go test ./internal/clients/weather_test.go

# Run a single test function
go test -run TestWeatherClientGetWeather ./internal/clients

# Run tests in a specific package
go test ./internal/clients
go test ./cmd/hass-dashboard

# Run tests with race detector
go test -race ./...

# Run tests with timeout
go test -timeout 30s ./...
```

### Lint
```bash
# Run golangci-lint (primary linter)
golangci-lint run

# Run with auto-fix where possible
golangci-lint run --fix

# Lint specific directory
golangci-lint run ./internal/...

# Lint with verbose output
golangci-lint run -v
```

### Format
```bash
# Format code with gofumpt (stricter than gofmt)
gofumpt -l -w .

# Organize imports with goimports
goimports -w -local github.com/markis/hass-dashboard .

# Standard go fmt
go fmt ./...
```

### Other Commands
```bash
# Run the application
./hass-dashboard --config config.yaml

# Tidy dependencies
go mod tidy

# Verify dependencies
go mod verify

# Download dependencies
go mod download
```

## Code Style Guidelines

### Formatting
- **Line Length**: 120 characters maximum
- **Indentation**: 2 spaces (tabs in Go files handled by gofmt)
- **Final Newline**: Always include
- **Trailing Whitespace**: Remove all
- **Line Endings**: LF (Unix-style)

### Imports
- Group imports in three sections: stdlib, external, local
- Local imports use prefix: `github.com/markis/hass-dashboard`
- Use `goimports` with `-local` flag to organize correctly
- No blank imports except for side-effects (document why with comment)
- Example:
  ```go
  import (
      "context"
      "fmt"
      
      "gopkg.in/yaml.v3"
      
      "github.com/markis/hass-dashboard/internal/clients"
  )
  ```

### Naming Conventions
- **Packages**: lowercase, single word when possible (e.g., `clients`, `models`, `render`)
- **Types**: PascalCase (e.g., `WeatherClient`, `Config`)
- **Functions/Methods**: camelCase for private, PascalCase for exported
- **Variables**: camelCase, meaningful names
- **Constants**: PascalCase or ALL_CAPS for exported
- **Minimum Variable Name Length**: 2 characters (exceptions: `i`, `j` for loops, `ok` for map checks)
- **Allowed Single Letters**: `i`, `j`, `id`, `tx`, `db`, `wg`, `ctx`, `err`, `ok`

### Types and Interfaces
- Always use explicit types for struct fields
- Use `context.Context` as first parameter in functions that need it
- Prefer concrete types over interfaces unless abstraction is needed
- Document all exported types, functions, and methods
- Use pointer receivers for methods that modify state
- Use value receivers for methods that don't modify state

### Error Handling
- **Always check errors** - blank error checks are not allowed
- **Wrap errors with context** using `fmt.Errorf` with `%w` verb:
  ```go
  return nil, fmt.Errorf("fetching calendar %s: %w", calID, err)
  ```
- **Do NOT use** `github.com/pkg/errors` - use stdlib `errors` package
- **Do NOT use** `io/ioutil` - use `io` and `os` packages
- Return early on error - avoid deep nesting
- Check type assertions: `value, ok := x.(Type)`
- Close resources properly - especially `io.Closer` types

### Functions
- **Maximum Lines**: 100 lines
- **Maximum Statements**: 50 statements
- **Cyclomatic Complexity**: Keep below 15
- **Cognitive Complexity**: Keep below 20
- Context as first parameter: `func DoSomething(ctx context.Context, ...)`
- Error as last return value: `func DoSomething(...) (result Type, err error)`

### Comments
- Document all exported types, functions, constants, and variables
- Start comments with the name of the thing being documented
- Use complete sentences with proper punctuation
- Example:
  ```go
  // WeatherClient fetches weather data from OpenWeatherMap.
  type WeatherClient struct { ... }
  
  // GetWeather retrieves current weather and forecast data.
  func (c *WeatherClient) GetWeather(ctx context.Context, ...) { ... }
  ```

### Testing
- Test files named `*_test.go`
- Use table-driven tests when testing multiple scenarios
- Use `t.Run()` for subtests with descriptive names
- Use `httptest.NewServer()` for testing HTTP clients
- Mock external dependencies
- Test error conditions, not just happy paths
- Example test names: `TestWeatherClientGetWeather`, `TestNewCalendarClient`

### Concurrency
- Use `sync.RWMutex` for shared state that's read often, written rarely
- Use `sync.Mutex` for simple exclusive access
- Always defer `Unlock()` immediately after `Lock()`
- Use channels for communication, mutexes for state
- Context for cancellation and timeouts

### Struct Tags
- YAML tags must match the external API/config format exactly
- Use lowercase with underscores for YAML: `yaml:"home_assistant"`
- JSON tags should be lowercase/camelCase as appropriate: `json:"calendarId"`

### Best Practices
- Use `time.Duration` constants: `30 * time.Second`, not magic numbers
- HTTP client timeouts are required (30s is standard for this project)
- Log errors before returning when appropriate
- Use meaningful variable names over comments
- Prefer small, focused functions over large ones
- Group related functionality in the same file
- Keep package-level docs in a separate `doc.go` file if extensive

## Project Structure

```
hass-dashboard/
├── cmd/
│   └── hass-dashboard/     # Main application entry point
├── internal/
│   ├── clients/            # HTTP clients (Calendar, Weather)
│   ├── models/             # Data models
│   └── render/             # HTML rendering and image generation
├── static/                 # Static assets (CSS, fonts, icons)
├── scripts/                # Shell scripts (healthcheck, utilities)
├── docs/                   # Documentation
├── config.yaml             # Runtime configuration
└── .golangci.yml           # Linter configuration
```

## Configuration Files

- `.golangci.yml`: Complete golangci-lint configuration with all rules
- `.editorconfig`: Editor formatting settings
- `go.mod`: Go module dependencies
- `config.yaml`: Application runtime configuration (not committed)
- `config.example.yaml`: Example configuration template

## Disabled Linters

The following linters are explicitly disabled (see `.golangci.yml` for reasons):
- `wrapcheck`, `exhaustruct`, `ireturn`, `err113`, `mnd`
- `noinlineerr`, `tagliatelle`, `wsl`, `noctx`, `perfsprint`

## Test Files Exemptions

Test files (`*_test.go`) are exempted from these linters:
- `cyclop`, `funlen`, `gocognit`, `goconst`, `dupl`
- `varnamelen`, `nestif`, `errcheck`, `paralleltest`
- `testpackage`, `govet`, `wsl_v5`, `nlreturn`

## Common Patterns in This Codebase

- HTTP clients use 30-second timeouts
- Caching with `sync.RWMutex` and TTL expiry (see `WeatherClient`)
- Stale cache fallback on API errors for resilience
- Context-aware operations with proper propagation
- YAML configuration with `gopkg.in/yaml.v3`
- Error wrapping with `fmt.Errorf` and `%w`
