// Package serve exposes the pre-rendered dashboard image over HTTP.
//
// Serving is read-only from disk: the request path never triggers dashboard
// generation, and the server only serves the artifact the generation loop has
// already written. Until the first render lands a request gets 404, which
// keeps an httpGet readiness probe on the served path red until the image is
// precomputed.
package serve

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// ShutdownTimeout bounds the graceful drain of in-flight serving requests on
// shutdown.
const ShutdownTimeout = 10 * time.Second

// Request timeout budget; 30s is the project-wide standard for HTTP.
const (
	readHeaderTimeout = 10 * time.Second
	bodyWriteTimeout  = 30 * time.Second
	idleTimeout       = 60 * time.Second
)

// spanName is the serving span name; it mirrors the "dashboard.generate"
// span from the generation loop as a sibling root trace.
const spanName = "dashboard.serve"

// Config controls serving the rendered dashboard image over HTTP.
type Config struct {
	// Enabled starts the read-only image server next to the generator.
	Enabled bool `yaml:"enabled"`
	// Listen is the [address]:port to bind. Non-root processes cannot bind
	// :80, so use a port above 1024 (default ":8080").
	Listen string `yaml:"listen"`
}

// Server serves the current dashboard image produced by the generation loop.
type Server struct {
	server *http.Server
	route  string
}

// New creates the image server for the given listen address and output image
// path. The image is served at "/" plus the basename of outputPath, e.g.
// "/output/1.jpg" is served at "/1.jpg".
func New(listen, outputPath string) (*Server, error) {
	handler, route, err := newHandler(outputPath)
	if err != nil {
		return nil, fmt.Errorf("building image handler: %w", err)
	}

	return &Server{
		server: &http.Server{
			Addr:              listen,
			Handler:           handler,
			ReadHeaderTimeout: readHeaderTimeout,
			WriteTimeout:      bodyWriteTimeout,
			IdleTimeout:       idleTimeout,
		},
		route: route,
	}, nil
}

// ListenAndServe binds the configured address and serves the image until
// Shutdown is called; run it from its own goroutine.
func (s *Server) ListenAndServe() error {
	return s.server.ListenAndServe()
}

// Shutdown gracefully drains in-flight serving requests bounded by ctx.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// Route is the URL path serving the image, e.g. "/1.jpg".
func (s *Server) Route() string {
	return s.route
}

// newHandler builds the fully instrumented HTTP handler for outputPath and
// returns it together with the route the image is exposed at.
func newHandler(outputPath string) (http.Handler, string, error) {
	route, err := routeForPath(outputPath)
	if err != nil {
		return nil, "", err
	}

	mux := http.NewServeMux()
	mux.Handle(route, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveImage(w, r, outputPath)
	}))

	// The middleware is the outermost handler so one serving span covers the
	// whole request with standard http.* attributes; generation stays a
	// separate root trace because the display polls on its own cadence and
	// never triggers rendering.
	return otelhttp.NewHandler(mux, spanName), route, nil
}

// routeForPath maps the output image path to a URL route. The filename must
// be URL-safe so that registering it as a ServeMux pattern cannot break the
// route or inject wildcard segments.
func routeForPath(outputPath string) (string, error) {
	name := filepath.Base(outputPath)
	if name == "." || name == "/" {
		return "", fmt.Errorf("%q has no image filename", outputPath)
	}

	if strings.IndexFunc(name, isUnsafeFilenameRune) != -1 {
		return "", fmt.Errorf(
			"%q: filename %q is not URL-safe (letters, digits, '.', '_', '-' only)", outputPath, name)
	}

	return "/" + name, nil
}

// isUnsafeFilenameRune reports whether r cannot appear in the image URL path.
func isUnsafeFilenameRune(r rune) bool {
	return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '.' && r != '_' && r != '-'
}

// serveImage writes the image file with Last-Modified/ETag semantics and the
// standard static-file behaviors (conditional GET 304s, Range, HEAD). It
// never generates anything: at worst it serves the previous artifact, and
// before the very first render the route just 404s.
func serveImage(res http.ResponseWriter, req *http.Request, outputPath string) {
	info, statErr := os.Stat(outputPath)
	if statErr != nil || info.IsDir() {
		http.NotFound(res, req)

		return
	}

	file, err := os.Open(outputPath) // #nosec G304 -- outputPath is the operator's config value
	if err != nil {
		log.Printf("Error serving dashboard image %s: %v", outputPath, err)
		http.Error(res, "dashboard image unavailable", http.StatusInternalServerError)

		return
	}
	defer func() {
		//nolint:errcheck // Best effort cleanup
		_ = file.Close()
	}()

	// The ETag (size + mtime, like nginx) must be set before ServeContent so
	// that If-None-Match is honored; together with the Last-Modified handling,
	// conditional GETs return 304 while the image is unchanged.
	res.Header().Set("ETag", etagFor(info))

	http.ServeContent(res, req, info.Name(), info.ModTime(), file)
}

// etagFor returns a weak entity tag for the file contents, stable across
// requests until the file is replaced by the generation loop.
func etagFor(info os.FileInfo) string {
	return fmt.Sprintf(`"%x-%x"`, info.ModTime().Unix(), info.Size())
}
