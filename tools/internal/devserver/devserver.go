// Package devserver implements `wpix serve`: a static file server for a
// browser game's development build that pushes asset changes to the open
// page.
//
// Usage:
//
//	wpix serve [--addr :8080] [--assets <dir>] <dir>
//
// It serves <dir> (the page and game.wasm) at /, and the --assets directory
// under /assets/, both with "Cache-Control: no-cache" so the browser
// revalidates instead of reusing a stale body after a save. GET
// /_weakpixel/changes is an event stream (server-sent events): the server
// polls the modtime and size of the asset tree every 250 ms, where a stat is
// cheap, and sends one `data: {"paths": [...]}` event per batch of changed
// files. Paths are asset keys: slash-separated, relative to the asset
// directory. A browser cannot list directories, so the server also answers
// /assets/assets.catalog.json from the asset directory (scanned on every
// request, so new files show after a page reload) unless the directory has
// its own. A game turns the stream into hot reload with
// httpfs.WatchChanges and game.Config.AssetChanges.
//
// Exit codes: 0 ok, 1 the server failed, 2 usage error.
package devserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/weakpixel/engine/asset/library"
	"github.com/weakpixel/engine/tools/internal/cli"
)

// ChangesPath is the URL path of the event stream.
const ChangesPath = "/_weakpixel/changes"

// pollInterval is how often the asset tree is scanned.
const pollInterval = 250 * time.Millisecond

// Options are the arguments of `wpix serve`.
type Options struct {
	Dir    string // the build directory, served at /
	Addr   string // address to listen on
	Assets string // asset directory, served under /assets/ and watched; may be empty
}

// Run serves opts.Dir until ctx is done and prints the address to stdout. A
// directory that does not exist is a usage error; a listener or server that
// fails is a plain error.
func Run(ctx context.Context, opts Options, stdout io.Writer) error {
	for _, d := range []string{opts.Dir, opts.Assets} {
		if d == "" {
			continue
		}
		if st, err := os.Stat(d); err != nil || !st.IsDir() {
			return cli.Usagef("%s is not a directory", d)
		}
	}
	ln, err := net.Listen("tcp", opts.Addr)
	if err != nil {
		return err
	}
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	s := New(ctx, opts.Dir, opts.Assets, pollInterval)
	srv := &http.Server{Handler: s.Handler()}
	go func() {
		<-ctx.Done()
		srv.Close()
	}()
	fmt.Fprintf(stdout, "serving %s on http://%s/\n", opts.Dir, ln.Addr())
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Server serves a build directory and an asset directory and pushes the
// asset directory's changes to event-stream clients.
type Server struct {
	dir, assets string

	mu   sync.Mutex
	subs map[chan []string]struct{}
}

// New returns a Server and, when assets is not empty, starts polling it
// every interval until ctx ends. The tree is scanned once before New
// returns, so only changes after that are reported.
func New(ctx context.Context, dir, assets string, interval time.Duration) *Server {
	s := &Server{dir: dir, assets: assets, subs: map[chan []string]struct{}{}}
	if assets != "" {
		prev := scan(assets)
		go func() {
			t := time.NewTicker(interval)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					cur := scan(assets)
					if changed := diff(prev, cur); len(changed) > 0 {
						s.publish(changed)
					}
					prev = cur
				}
			}
		}()
	}
	return s
}

// Handler returns the HTTP handler: the build at /, the assets under
// /assets/ and the event stream at [ChangesPath].
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir(s.dir)))
	if s.assets != "" {
		mux.HandleFunc("/assets/"+library.CatalogFile, s.catalog)
		mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir(s.assets))))
	}
	mux.HandleFunc(ChangesPath, s.stream)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		mux.ServeHTTP(w, r)
	})
}

// catalog answers the library catalog of the asset directory: its own
// file if it has one, else the catalog of a scan with loose files.
func (s *Server) catalog(w http.ResponseWriter, r *http.Request) {
	if _, err := os.Stat(filepath.Join(s.assets, library.CatalogFile)); err == nil {
		http.ServeFile(w, r, filepath.Join(s.assets, library.CatalogFile))
		return
	}
	cat, _, err := library.Scan(os.DirFS(s.assets))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(cat.Encode())
}

// stream is the event-stream handler: one `data:` line per published batch,
// until the client leaves.
func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	ch := make(chan []string, 16)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.subs, ch)
		s.mu.Unlock()
	}()
	w.Header().Set("Content-Type", "text/event-stream")
	// A comment line makes the headers reach the client now, so it knows
	// the stream is up before the first change.
	fmt.Fprint(w, ": connected\n\n")
	fl.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case paths := <-ch:
			data, _ := json.Marshal(struct {
				Paths []string `json:"paths"`
			}{paths})
			fmt.Fprintf(w, "data: %s\n\n", data)
			fl.Flush()
		}
	}
}

// publish sends one batch to every client. A client that is 16 batches
// behind loses this one rather than stalling the others.
func (s *Server) publish(paths []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for ch := range s.subs {
		select {
		case ch <- paths:
		default:
		}
	}
}

// stamp identifies a file version for change detection.
type stamp struct {
	mod  time.Time
	size int64
}

// scan returns the stamp of every regular file under root, by asset key.
// Unreadable entries are skipped: a file mid-save shows up on a later scan.
func scan(root string) map[string]stamp {
	out := map[string]stamp{}
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return nil
		}
		out[filepath.ToSlash(rel)] = stamp{info.ModTime(), info.Size()}
		return nil
	})
	return out
}

// diff returns the sorted keys that are new or changed in cur, plus those
// removed since prev (the game reports a vanished file as a reload error).
func diff(prev, cur map[string]stamp) []string {
	var changed []string
	for k, v := range cur {
		if p, ok := prev[k]; !ok || p != v {
			changed = append(changed, k)
		}
	}
	for k := range prev {
		if _, ok := cur[k]; !ok {
			changed = append(changed, k)
		}
	}
	slices.Sort(changed)
	return changed
}
