package server

import (
	"bytes"
	"compress/gzip"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"

	"github.com/andybalholm/brotli"
)

var compressibleTestContent = bytes.Repeat([]byte("gosherve static content\n"), 256)

func TestPrepareStaticFiles(t *testing.T) {
	webroot := fstest.MapFS{
		"index.html":      {Data: compressibleTestContent},
		"docs/index.html": {Data: []byte("documentation")},
	}

	prepared, err := PrepareStaticFiles(webroot)
	if err != nil {
		t.Fatal(err)
	}
	fsys := fs.FS(webroot)
	ordinaryServer := NewServer(&fsys, "")
	if ordinaryServer.staticFiles != nil {
		t.Fatal("NewServer enabled prepared files without WithStaticFiles")
	}
	server := NewServer(&fsys, "", WithStaticFiles(prepared))
	if server.staticFiles == nil {
		t.Fatal("WithStaticFiles did not enable prepared serving")
	}

	tests := []struct {
		path string
		body []byte
	}{
		{path: "/", body: compressibleTestContent},
		{path: "/docs", body: []byte("documentation")},
		{path: "/docs/", body: []byte("documentation")},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			response := requestStaticFile(server, http.MethodGet, test.path, "", "")
			if response.Code != http.StatusOK {
				t.Fatalf("got status %d, want %d", response.Code, http.StatusOK)
			}
			if !bytes.Equal(response.Body.Bytes(), test.body) {
				t.Fatalf("got body %q, want %q", response.Body.Bytes(), test.body)
			}
			if response.Header().Get("ETag") == "" {
				t.Fatal("response did not include a precomputed ETag")
			}
		})
	}
}

func TestStaticFilesCanonicalIndexRedirect(t *testing.T) {
	webroot := fstest.MapFS{"index.html": {Data: compressibleTestContent}}
	prepared, err := PrepareStaticFiles(webroot, WithPrecompression(CompressionGzip, CompressionBrotli))
	if err != nil {
		t.Fatal(err)
	}
	fsys := fs.FS(webroot)
	servers := map[string]*Server{
		"standard": NewServer(&fsys, ""),
		"prepared": NewServer(&fsys, "", WithStaticFiles(prepared)),
	}
	for name, server := range servers {
		t.Run(name, func(t *testing.T) {
			response := requestStaticFile(server, http.MethodGet, "/index.html?source=test", "br", "")
			if response.Code != http.StatusMovedPermanently {
				t.Fatalf("got status %d, want 301", response.Code)
			}
			if got := response.Header().Get("Location"); got != "./?source=test" {
				t.Fatalf("got Location %q, want %q", got, "./?source=test")
			}
			if response.Header().Get("ETag") == "" {
				t.Fatal("redirect did not retain the file ETag")
			}
		})
	}
}

func TestStaticFilesCompressionNegotiation(t *testing.T) {
	server := newCompressedTestServer(t)
	tests := []struct {
		name           string
		acceptEncoding string
		wantEncoding   string
		wantStatus     int
	}{
		{name: "no header", wantEncoding: identityEncoding, wantStatus: http.StatusOK},
		{name: "gzip", acceptEncoding: "gzip", wantEncoding: "gzip", wantStatus: http.StatusOK},
		{name: "brotli", acceptEncoding: "br", wantEncoding: "br", wantStatus: http.StatusOK},
		{name: "server preference", acceptEncoding: "gzip, br", wantEncoding: "br", wantStatus: http.StatusOK},
		{name: "client preference", acceptEncoding: "gzip;q=1, br;q=0.5", wantEncoding: "gzip", wantStatus: http.StatusOK},
		{name: "identity preference", acceptEncoding: "br;q=0.5", wantEncoding: identityEncoding, wantStatus: http.StatusOK},
		{name: "wildcard", acceptEncoding: "*", wantEncoding: "br", wantStatus: http.StatusOK},
		{name: "explicit exclusion", acceptEncoding: "br;q=0, gzip;q=1", wantEncoding: "gzip", wantStatus: http.StatusOK},
		{name: "compressed excluded", acceptEncoding: "br;q=0, gzip;q=0", wantEncoding: identityEncoding, wantStatus: http.StatusOK},
		{name: "nothing acceptable", acceptEncoding: "identity;q=0, br;q=0, gzip;q=0", wantStatus: http.StatusNotAcceptable},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := requestStaticFile(server, http.MethodGet, "/", test.acceptEncoding, "")
			if response.Code != test.wantStatus {
				t.Fatalf("got status %d, want %d", response.Code, test.wantStatus)
			}
			if test.wantStatus != http.StatusOK {
				return
			}

			gotEncoding := response.Header().Get("Content-Encoding")
			if test.wantEncoding == identityEncoding {
				if gotEncoding != "" {
					t.Fatalf("got encoding %q, want identity", gotEncoding)
				}
			} else if gotEncoding != test.wantEncoding {
				t.Fatalf("got encoding %q, want %q", gotEncoding, test.wantEncoding)
			}
			if got := response.Header().Get("Vary"); got != "Accept-Encoding" {
				t.Fatalf("got Vary %q, want Accept-Encoding", got)
			}
			if got := decodeStaticBody(t, response, test.wantEncoding); !bytes.Equal(got, compressibleTestContent) {
				t.Fatal("decoded response did not match source content")
			}
		})
	}
	if got := readCounterVec(*server.metrics.responseStatus, "406"); got != 1 {
		t.Fatalf("got 406 metric %v, want 1", got)
	}
}

func TestStaticFilesRepresentationETags(t *testing.T) {
	server := newCompressedTestServer(t)
	etags := map[string]string{}
	for _, encoding := range []string{identityEncoding, "gzip", "br"} {
		acceptEncoding := encoding
		if encoding == identityEncoding {
			acceptEncoding = "identity"
		}
		response := requestStaticFile(server, http.MethodGet, "/", acceptEncoding, "")
		if response.Code != http.StatusOK {
			t.Fatalf("%s response returned status %d", encoding, response.Code)
		}
		etags[encoding] = response.Header().Get("ETag")
		if etags[encoding] == "" {
			t.Fatalf("%s response did not include an ETag", encoding)
		}
	}
	if etags[identityEncoding] == etags["gzip"] || etags[identityEncoding] == etags["br"] || etags["gzip"] == etags["br"] {
		t.Fatalf("representation ETags are not distinct: %#v", etags)
	}

	for _, encoding := range []string{identityEncoding, "gzip", "br"} {
		acceptEncoding := encoding
		if encoding == identityEncoding {
			acceptEncoding = "identity"
		}
		response := requestStaticFile(server, http.MethodGet, "/", acceptEncoding, etags[encoding])
		if response.Code != http.StatusNotModified {
			t.Fatalf("%s conditional response returned status %d, want 304", encoding, response.Code)
		}
		if response.Body.Len() != 0 {
			t.Fatalf("%s conditional response included a body", encoding)
		}
	}

	response := requestStaticFile(server, http.MethodGet, "/", "br", etags["gzip"])
	if response.Code != http.StatusOK {
		t.Fatalf("validator for a different representation returned status %d, want 200", response.Code)
	}
	if got := readCounterVec(*server.metrics.responseStatus, "304"); got != 3 {
		t.Fatalf("got 304 metric %v, want 3", got)
	}
}

func TestStaticFilesRangeRequest(t *testing.T) {
	server := newCompressedTestServer(t)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Accept-Encoding", "identity")
	request.Header.Set("Range", "bytes=0-9")
	response := httptest.NewRecorder()
	server.routeHandler(response, request)

	if response.Code != http.StatusPartialContent {
		t.Fatalf("got status %d, want 206", response.Code)
	}
	if got := response.Header().Get("Content-Range"); got != "bytes 0-9/6144" {
		t.Fatalf("got Content-Range %q", got)
	}
	if !bytes.Equal(response.Body.Bytes(), compressibleTestContent[:10]) {
		t.Fatal("range response did not contain the requested bytes")
	}
	if got := readCounterVec(*server.metrics.responseStatus, "206"); got != 1 {
		t.Fatalf("got 206 metric %v, want 1", got)
	}
}

func TestStaticFilesCompressionEligibility(t *testing.T) {
	webroot := fstest.MapFS{
		"large.html": {Data: compressibleTestContent},
		"small.txt":  {Data: []byte("small")},
		"image.png":  {Data: bytes.Repeat([]byte{0x89, 'P', 'N', 'G'}, 1024)},
		"dense.txt":  {Data: []byte("x")},
	}
	prepared, err := PrepareStaticFiles(
		webroot,
		WithPrecompression(CompressionGzip, CompressionBrotli),
		WithMinimumCompressionSize(16),
	)
	if err != nil {
		t.Fatal(err)
	}
	manifest := prepared.manifest

	if len(manifest.files["large.html"].representations) != 2 {
		t.Fatalf("large text file has %d representations, want 2", len(manifest.files["large.html"].representations))
	}
	if len(manifest.files["small.txt"].representations) != 0 {
		t.Fatal("file below the compression threshold was compressed")
	}
	if len(manifest.files["image.png"].representations) != 0 {
		t.Fatal("already-compressed media type was compressed")
	}

	prepared, err = PrepareStaticFiles(
		fstest.MapFS{"dense.txt": {Data: []byte("x")}},
		WithPrecompression(CompressionGzip, CompressionBrotli),
		WithMinimumCompressionSize(0),
	)
	if err != nil {
		t.Fatal(err)
	}
	manifest = prepared.manifest
	if len(manifest.files["dense.txt"].representations) != 0 {
		t.Fatal("compressed representation larger than its source was retained")
	}
}

func TestStaticFilesPreparedNotFound(t *testing.T) {
	webroot := fstest.MapFS{
		"404.html": {Data: bytes.Repeat([]byte("not found "), 256)},
	}
	prepared, err := PrepareStaticFiles(webroot, WithPrecompression(CompressionGzip, CompressionBrotli))
	if err != nil {
		t.Fatal(err)
	}
	fsys := fs.FS(webroot)
	server := NewServer(&fsys, "", WithStaticFiles(prepared), WithCacheRules(
		CacheRule{Pattern: "404.html", CacheControl: "no-store"},
	))

	response := requestStaticFile(server, http.MethodGet, "/missing", "br", "")
	if response.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want 404", response.Code)
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("got Cache-Control %q, want no-store", got)
	}
	if got := response.Header().Get("Content-Encoding"); got != "br" {
		t.Fatalf("got Content-Encoding %q, want br", got)
	}
	if got := decodeStaticBody(t, response, "br"); !bytes.Equal(got, webroot["404.html"].Data) {
		t.Fatal("decoded 404 response did not match source content")
	}

	etag := response.Header().Get("ETag")
	conditional := requestStaticFile(server, http.MethodGet, "/missing", "br", etag)
	if conditional.Code != http.StatusNotModified {
		t.Fatalf("conditional 404 returned status %d, want 304", conditional.Code)
	}
	if conditional.Body.Len() != 0 {
		t.Fatal("conditional 404 included a body")
	}
	if got := readCounterVec(*server.metrics.responseStatus, "404"); got != 1 {
		t.Fatalf("got 404 metric %v, want 1", got)
	}
	if got := readCounterVec(*server.metrics.responseStatus, "304"); got != 1 {
		t.Fatalf("got 304 metric %v, want 1", got)
	}
}

func TestStaticFilesHeadAndConditionalRequestsDoNotReadContent(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "index.html"), compressibleTestContent, 0o600); err != nil {
		t.Fatal(err)
	}
	counted := &countingFS{FS: os.DirFS(directory)}
	prepared, err := PrepareStaticFiles(counted, WithPrecompression(CompressionGzip, CompressionBrotli))
	if err != nil {
		t.Fatal(err)
	}
	fsys := fs.FS(counted)
	server := NewServer(&fsys, "", WithStaticFiles(prepared))
	etag := server.staticFiles.files["index.html"].etag

	counted.bytesRead.Store(0)
	head := requestStaticFile(server, http.MethodHead, "/", "identity", "")
	if head.Code != http.StatusOK {
		t.Fatalf("HEAD returned status %d", head.Code)
	}
	if head.Body.Len() != 0 {
		t.Fatal("HEAD response included a body")
	}
	if got := counted.bytesRead.Load(); got != 0 {
		t.Fatalf("HEAD read %d content bytes, want 0", got)
	}

	counted.bytesRead.Store(0)
	conditional := requestStaticFile(server, http.MethodGet, "/", "identity", etag)
	if conditional.Code != http.StatusNotModified {
		t.Fatalf("conditional request returned status %d, want 304", conditional.Code)
	}
	if got := counted.bytesRead.Load(); got != 0 {
		t.Fatalf("conditional request read %d content bytes, want 0", got)
	}
}

func TestStaticFilesConcurrentServing(t *testing.T) {
	server := newCompressedTestServer(t)
	var workers sync.WaitGroup
	for range 32 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			response := requestStaticFile(server, http.MethodGet, "/", "gzip, br", "")
			if response.Code != http.StatusOK {
				t.Errorf("got status %d, want 200", response.Code)
			}
		}()
	}
	workers.Wait()
}

func TestStaticFilesOptions(t *testing.T) {
	tests := []struct {
		name string
		opt  StaticFilesOption
	}{
		{name: "unknown encoding", opt: WithPrecompression(CompressionEncoding("zstd"))},
		{name: "negative threshold", opt: WithMinimumCompressionSize(-1)},
		{name: "invalid gzip level", opt: WithGzipLevel(10)},
		{name: "invalid Brotli level", opt: WithBrotliLevel(12)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := PrepareStaticFiles(fstest.MapFS{}, test.opt); err == nil {
				t.Fatal("PrepareStaticFiles accepted invalid configuration")
			}
		})
	}
	if _, err := PrepareStaticFiles(nil); err == nil {
		t.Fatal("PrepareStaticFiles accepted a nil filesystem")
	}
	server := NewServer(nil, "", WithStaticFiles(nil))
	if server.staticFiles != nil {
		t.Fatal("WithStaticFiles(nil) enabled prepared serving")
	}
}

func BenchmarkStaticFilesServing(b *testing.B) {
	originalLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	b.Cleanup(func() { slog.SetDefault(originalLogger) })

	webroot := fstest.MapFS{"index.html": {Data: bytes.Repeat([]byte("benchmark content\n"), 4096)}}
	prepared, err := PrepareStaticFiles(webroot, WithPrecompression(CompressionGzip, CompressionBrotli))
	if err != nil {
		b.Fatal(err)
	}
	standardFS := fs.FS(webroot)
	servers := map[string]*Server{
		"standard": NewServer(&standardFS, ""),
		"prepared": NewServer(&standardFS, "", WithStaticFiles(prepared)),
	}

	for name, server := range servers {
		b.Run(name+"/GET", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				requestStaticFile(server, http.MethodGet, "/", "identity", "")
			}
		})

		etag := calculateETag("index.html", &standardFS)
		if server.staticFiles != nil {
			etag = server.staticFiles.files["index.html"].etag
		}
		b.Run(name+"/304", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				requestStaticFile(server, http.MethodGet, "/", "identity", etag)
			}
		})
	}

	for _, encoding := range []string{"gzip", "br"} {
		b.Run("prepared/GET/"+encoding, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				requestStaticFile(servers["prepared"], http.MethodGet, "/", encoding, "")
			}
		})
	}
}

func BenchmarkStaticFilesPreparation(b *testing.B) {
	webroot := fstest.MapFS{
		"index.html": {Data: bytes.Repeat([]byte("benchmark content\n"), 65536)},
		"image.png":  {Data: bytes.Repeat([]byte{0x89, 'P', 'N', 'G'}, 1<<20)},
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := PrepareStaticFiles(webroot, WithPrecompression(CompressionGzip, CompressionBrotli)); err != nil {
			b.Fatal(err)
		}
	}
}

func newCompressedTestServer(t *testing.T) *Server {
	t.Helper()
	webroot := fstest.MapFS{"index.html": {Data: compressibleTestContent}}
	prepared, err := PrepareStaticFiles(webroot, WithPrecompression(CompressionGzip, CompressionBrotli))
	if err != nil {
		t.Fatal(err)
	}
	fsys := fs.FS(webroot)
	return NewServer(&fsys, "", WithStaticFiles(prepared))
}

func requestStaticFile(server *Server, method, path, acceptEncoding, etag string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, nil)
	if acceptEncoding != "" {
		request.Header.Set("Accept-Encoding", acceptEncoding)
	}
	if etag != "" {
		request.Header.Set("If-None-Match", etag)
	}
	response := httptest.NewRecorder()
	server.routeHandler(response, request)
	return response
}

func decodeStaticBody(t *testing.T, response *httptest.ResponseRecorder, encoding string) []byte {
	t.Helper()
	var reader io.Reader = response.Body
	switch encoding {
	case identityEncoding:
		return response.Body.Bytes()
	case "gzip":
		gzipReader, err := gzip.NewReader(reader)
		if err != nil {
			t.Fatal(err)
		}
		defer gzipReader.Close()
		reader = gzipReader
	case "br":
		reader = brotli.NewReader(reader)
	default:
		t.Fatalf("unsupported test encoding %q", encoding)
	}
	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

type countingFS struct {
	fs.FS
	bytesRead atomic.Int64
}

func (counted *countingFS) Open(name string) (fs.File, error) {
	file, err := counted.FS.Open(name)
	if err != nil {
		return nil, err
	}
	return &countingFile{File: file, bytesRead: &counted.bytesRead}, nil
}

type countingFile struct {
	fs.File
	bytesRead *atomic.Int64
}

func (file *countingFile) Read(buffer []byte) (int, error) {
	count, err := file.File.Read(buffer)
	file.bytesRead.Add(int64(count))
	return count, err
}

func (file *countingFile) ReadDir(count int) ([]fs.DirEntry, error) {
	return file.File.(fs.ReadDirFile).ReadDir(count)
}

func (file *countingFile) Seek(offset int64, whence int) (int64, error) {
	return file.File.(io.Seeker).Seek(offset, whence)
}
