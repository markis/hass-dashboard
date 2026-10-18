package serve

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeTempImage writes image content to a temp file and returns its path
// plus modification time.
func writeTempImage(t *testing.T, content string) (string, time.Time) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "1.jpg")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing temp image: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stating temp image: %v", err)
	}

	return path, info.ModTime()
}

func TestRouteForPath(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		route   string
		wantErr bool
	}{
		{name: "simple filename", path: "output.jpg", route: "/output.jpg"},
		{name: "absolute path", path: "/output/1.jpg", route: "/1.jpg"},
		{name: "trailing slash", path: "/output/1.jpg/", route: "/1.jpg"},
		{name: "png with dots", path: "/var/www/dashboard-v1.2.png", route: "/dashboard-v1.2.png"},
		{name: "empty path", path: "", wantErr: true},
		{name: "root path", path: "/", wantErr: true},
		{name: "wildcard marker", path: "/output/dash{1}.jpg", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			route, err := routeForPath(tc.path)
			if tc.wantErr {
				if err == nil {
					t.Errorf("routeForPath(%q) = %q, want error", tc.path, route)
				}

				return
			}

			if err != nil {
				t.Errorf("routeForPath(%q) error: %v", tc.path, err)
			}

			if route != tc.route {
				t.Errorf("routeForPath(%q) = %q, want %q", tc.path, route, tc.route)
			}
		})
	}
}

func TestNewHandlerInvalidPath(t *testing.T) {
	if _, _, err := newHandler("/"); err == nil {
		t.Error("newHandler(\"/\") error = nil, want error")
	}
}

func TestServeImageGet(t *testing.T) {
	const content = "fake-jpeg-bytes"

	path, modTime := writeTempImage(t, content)

	handler, route, err := newHandler(path)
	if err != nil {
		t.Fatalf("newHandler error: %v", err)
	}

	if route != "/1.jpg" {
		t.Errorf("route = %q, want /1.jpg", route)
	}

	req := httptest.NewRequest(http.MethodGet, "/1.jpg", http.NoBody)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusOK)
	}

	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "image/jpeg") {
		t.Errorf("Content-Type = %q, want image/jpeg", ct)
	}

	wantLM := modTime.Truncate(time.Second).UTC().Format(http.TimeFormat)
	if lm := res.Header.Get("Last-Modified"); lm != wantLM {
		t.Errorf("Last-Modified = %q, want %q", lm, wantLM)
	}

	if etag := res.Header.Get("ETag"); etag != etagFor(mustStat(t, path)) {
		t.Errorf("ETag = %q, want %q", etag, etagFor(mustStat(t, path)))
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}

	if string(body) != content {
		t.Errorf("body = %q, want %q", body, content)
	}
}

// mustStat stats a file and fails the test on error.
func mustStat(t *testing.T, path string) os.FileInfo {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stating %s: %v", path, err)
	}

	return info
}

func TestServeImageConditional304(t *testing.T) {
	const content = "conditional-jpeg"

	path, modTime := writeTempImage(t, content)

	handler, _, err := newHandler(path)
	if err != nil {
		t.Fatalf("newHandler error: %v", err)
	}

	etag := etagFor(mustStat(t, path))

	cases := []struct {
		name    string
		headers map[string]string
	}{
		{
			name: "If-Modified-Since matching mtime",
			headers: map[string]string{
				"If-Modified-Since": modTime.Truncate(time.Second).UTC().Format(http.TimeFormat),
			},
		},
		{
			name:    "If-None-Match matching ETag",
			headers: map[string]string{"If-None-Match": etag},
		},
		{
			name: "both conditional headers",
			headers: map[string]string{
				"If-Modified-Since": modTime.Truncate(time.Second).UTC().Format(http.TimeFormat),
				"If-None-Match":     etag,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/1.jpg", http.NoBody)
			for key, value := range tc.headers {
				req.Header.Set(key, value)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			res := rec.Result()
			defer res.Body.Close()

			if res.StatusCode != http.StatusNotModified {
				t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusNotModified)
			}

			body, _ := io.ReadAll(res.Body)
			if len(body) != 0 {
				t.Errorf("304 body = %q, want empty", body)
			}
		})
	}
}

func TestServeImageConditionalChanged(t *testing.T) {
	path, _ := writeTempImage(t, "new-content")

	handler, _, err := newHandler(path)
	if err != nil {
		t.Fatalf("newHandler error: %v", err)
	}

	// A stale ETag (e.g. cached before the image changed) must not 304.
	req := httptest.NewRequest(http.MethodGet, "/1.jpg", http.NoBody)
	req.Header.Set("If-None-Match", `"00000000-0"`)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d (non-matching conditionals must serve)", res.StatusCode, http.StatusOK)
	}
}

func TestServeImageRange(t *testing.T) {
	const content = "range-request-jpeg"

	path, _ := writeTempImage(t, content)

	handler, _, err := newHandler(path)
	if err != nil {
		t.Fatalf("newHandler error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/1.jpg", http.NoBody)
	req.Header.Set("Range", "bytes=0-4")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusPartialContent)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}

	if string(body) != content[:5] {
		t.Errorf("body = %q, want %q", body, content[:5])
	}

	if cr := res.Header.Get("Content-Range"); !strings.HasSuffix(cr, fmt.Sprintf("/%d", len(content))) {
		t.Errorf("Content-Range = %q, want total %d", cr, len(content))
	}
}

func TestServeImageHead(t *testing.T) {
	path, modTime := writeTempImage(t, "head-request")

	handler, _, err := newHandler(path)
	if err != nil {
		t.Fatalf("newHandler error: %v", err)
	}

	req := httptest.NewRequest(http.MethodHead, "/1.jpg", http.NoBody)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusOK)
	}

	if lm := res.Header.Get("Last-Modified"); lm != modTime.Truncate(time.Second).UTC().Format(http.TimeFormat) {
		t.Errorf("Last-Modified = %q, want mtime", lm)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}

	if len(body) != 0 {
		t.Errorf("HEAD body = %q, want empty", body)
	}
}

func TestServeImageMissingFile(t *testing.T) {
	// Nothing written yet — as before the first render lands.
	handler, _, err := newHandler(filepath.Join(t.TempDir(), "1.jpg"))
	if err != nil {
		t.Fatalf("newHandler error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/1.jpg", http.NoBody)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestOnlyImageRouteServed(t *testing.T) {
	path, _ := writeTempImage(t, "route-isolated")

	handler, route, err := newHandler(path)
	if err != nil {
		t.Fatalf("newHandler error: %v", err)
	}

	if route != "/1.jpg" {
		t.Fatalf("route = %q, want /1.jpg", route)
	}

	// Only the image itself may be fetched: no directory listing (autoindex
	// is gone) and no access to sidecar files like the .hash state.
	for _, target := range []string{"/", "/1.jpg.hash", "/other.jpg", "/1.jpg/"} {
		req := httptest.NewRequest(http.MethodGet, target, http.NoBody)

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s status = %d, want %d", target, rec.Code, http.StatusNotFound)
		}
	}
}
