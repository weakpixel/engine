package devserver

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/weakpixel/engine/asset/library"
	"github.com/weakpixel/engine/tools/internal/cli"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newTestServer(t *testing.T) (srv *httptest.Server, build, assets string) {
	t.Helper()
	build, assets = t.TempDir(), t.TempDir()
	write(t, filepath.Join(build, "index.html"), "<html>")
	write(t, filepath.Join(assets, "fx", "a.kage"), "old")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	srv = httptest.NewServer(New(ctx, build, assets, 10*time.Millisecond).Handler())
	t.Cleanup(srv.Close)
	return srv, build, assets
}

func TestServesWithNoCache(t *testing.T) {
	srv, _, _ := newTestServer(t)
	for path, want := range map[string]string{"/index.html": "<html>", "/assets/fx/a.kage": "old"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		var b bytes.Buffer
		b.ReadFrom(resp.Body)
		resp.Body.Close()
		if b.String() != want || resp.Header.Get("Cache-Control") != "no-cache" {
			t.Errorf("%s: body %q, Cache-Control %q", path, b.String(), resp.Header.Get("Cache-Control"))
		}
	}
}

func TestCatalogOfLooseFiles(t *testing.T) {
	srv, _, assets := newTestServer(t)
	write(t, filepath.Join(assets, "global", "fx", "s.kage"), "package main")
	resp, err := http.Get(srv.URL + "/assets/assets.catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	b.ReadFrom(resp.Body)
	resp.Body.Close()
	cat, err := library.ReadCatalog(b.Bytes())
	if err != nil {
		t.Fatalf("%v in %s", err, b.String())
	}
	if len(cat.Packages) == 0 || cat.Packages[0].Pack != "" {
		t.Fatalf("want loose packages, got %+v", cat.Packages)
	}
}

func TestChangeArrivesAsOneEvent(t *testing.T) {
	srv, _, assets := newTestServer(t)
	resp, err := http.Get(srv.URL + ChangesPath)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	lines := make(chan string, 8)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
	}()
	if l := <-lines; !strings.HasPrefix(l, ":") {
		t.Fatalf("first line %q, want the connect comment", l)
	}
	write(t, filepath.Join(assets, "fx", "a.kage"), "newer content")
	write(t, filepath.Join(assets, "b.png"), "png")
	var got []string
	deadline := time.After(5 * time.Second)
	for len(got) == 0 || !strings.Contains(strings.Join(got, ""), "b.png") {
		select {
		case l := <-lines:
			if strings.HasPrefix(l, "data:") {
				got = append(got, l)
			}
		case <-deadline:
			t.Fatalf("events so far: %v", got)
		}
	}
	all := strings.Join(got, "\n")
	if !strings.Contains(all, `"fx/a.kage"`) || !strings.Contains(all, `"b.png"`) {
		t.Fatalf("events %v do not name both files by asset key", got)
	}
}

func TestRunUsage(t *testing.T) {
	var u *cli.UsageError
	err := Run(context.Background(), Options{Dir: filepath.Join(t.TempDir(), "nope"), Addr: ":0"}, &bytes.Buffer{})
	if !errors.As(err, &u) {
		t.Fatalf("missing dir: %v", err)
	}
	err = Run(context.Background(), Options{Dir: t.TempDir(), Addr: ":0", Assets: filepath.Join(t.TempDir(), "nope")}, &bytes.Buffer{})
	if !errors.As(err, &u) {
		t.Fatalf("missing assets dir: %v", err)
	}
}

// TestRunStopsWithItsContext: Run serves until the context is done.
func TestRunStopsWithItsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var out syncBuffer
	done := make(chan error, 1)
	go func() { done <- Run(ctx, Options{Dir: t.TempDir(), Addr: "127.0.0.1:0"}, &out) }()
	deadline := time.After(5 * time.Second)
	for !strings.Contains(out.String(), "serving ") {
		select {
		case <-deadline:
			t.Fatal("the server did not start")
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-deadline:
		t.Fatal("Run did not return after cancel")
	}
}

// syncBuffer is a bytes.Buffer safe for the server goroutine to write while
// the test reads.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}
