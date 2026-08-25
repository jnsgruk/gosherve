package server

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/andybalholm/brotli"
)

// CompressionEncoding identifies an encoding that PrepareStaticFiles can prepare.
type CompressionEncoding string

const (
	// CompressionGzip prepares gzip representations.
	CompressionGzip CompressionEncoding = "gzip"
	// CompressionBrotli prepares Brotli representations.
	CompressionBrotli CompressionEncoding = "br"
)

const (
	defaultMinimumCompressionSize int64 = 1024
	defaultGzipLevel                    = 6
	defaultBrotliLevel                  = 5
	identityEncoding                    = "identity"
)

type staticFSConfig struct {
	encodings              map[CompressionEncoding]bool
	minimumCompressionSize int64
	gzipLevel              int
	brotliLevel            int
}

// StaticFilesOption configures prepared immutable files.
type StaticFilesOption func(*staticFSConfig) error

// WithPrecompression configures representations to prepare before serving.
func WithPrecompression(encodings ...CompressionEncoding) StaticFilesOption {
	return func(config *staticFSConfig) error {
		for _, encoding := range encodings {
			switch encoding {
			case CompressionGzip, CompressionBrotli:
				config.encodings[encoding] = true
			default:
				return fmt.Errorf("unsupported compression encoding %q", encoding)
			}
		}
		return nil
	}
}

// WithMinimumCompressionSize sets the smallest file that may be compressed.
func WithMinimumCompressionSize(size int64) StaticFilesOption {
	return func(config *staticFSConfig) error {
		if size < 0 {
			return fmt.Errorf("minimum compression size cannot be negative")
		}
		config.minimumCompressionSize = size
		return nil
	}
}

// WithGzipLevel sets the gzip compression level. Valid levels range from 0 to
// 9, along with the values supported by compress/gzip for default and Huffman-only compression.
func WithGzipLevel(level int) StaticFilesOption {
	return func(config *staticFSConfig) error {
		if level < gzip.HuffmanOnly || level > gzip.BestCompression {
			return fmt.Errorf("invalid gzip compression level %d", level)
		}
		config.gzipLevel = level
		return nil
	}
}

// WithBrotliLevel sets the Brotli compression level, from 0 to 11.
func WithBrotliLevel(level int) StaticFilesOption {
	return func(config *staticFSConfig) error {
		if level < 0 || level > 11 {
			return fmt.Errorf("invalid Brotli compression level %d", level)
		}
		config.brotliLevel = level
		return nil
	}
}

type staticRepresentation struct {
	content []byte
	etag    string
}

type staticFile struct {
	contentType     string
	etag            string
	modTime         time.Time
	path            string
	representations map[string]staticRepresentation
}

type staticManifest struct {
	files  map[string]*staticFile
	source fs.FS
}

func (manifest *staticManifest) resolve(requestPath string) (*staticFile, bool) {
	filepath := strings.Trim(requestPath, "/")
	if filepath == "" {
		filepath = "index.html"
	}

	if file, ok := manifest.files[filepath]; ok {
		return file, true
	}

	file, ok := manifest.files[path.Join(filepath, "index.html")]
	return file, ok
}

// StaticFiles contains an immutable manifest prepared for efficient serving.
// Construct one with PrepareStaticFiles and attach it with WithStaticFiles.
type StaticFiles struct {
	manifest *staticManifest
}

// PrepareStaticFiles prepares an immutable filesystem for efficient serving.
// It precomputes file metadata and content-based ETags, and optionally prepares
// compressed representations. Prepare files after applying fs.Sub.
func PrepareStaticFiles(fsys fs.FS, opts ...StaticFilesOption) (*StaticFiles, error) {
	if fsys == nil {
		return nil, fmt.Errorf("static filesystem cannot be nil")
	}

	config := staticFSConfig{
		encodings:              map[CompressionEncoding]bool{},
		minimumCompressionSize: defaultMinimumCompressionSize,
		gzipLevel:              defaultGzipLevel,
		brotliLevel:            defaultBrotliLevel,
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(&config); err != nil {
			return nil, err
		}
	}

	manifest := &staticManifest{
		files:  map[string]*staticFile{},
		source: fsys,
	}

	err := fs.WalkDir(fsys, ".", func(filepath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}

		info, err := fs.Stat(fsys, filepath)
		if err != nil {
			return fmt.Errorf("stat static file %q: %w", filepath, err)
		}
		content, err := fs.ReadFile(fsys, filepath)
		if err != nil {
			return fmt.Errorf("read static file %q: %w", filepath, err)
		}

		file := &staticFile{
			contentType:     detectContentType(filepath, content),
			etag:            contentETag(content),
			modTime:         info.ModTime(),
			path:            filepath,
			representations: map[string]staticRepresentation{},
		}

		if int64(len(content)) >= config.minimumCompressionSize && isCompressible(file.contentType) {
			if config.encodings[CompressionGzip] {
				compressed, err := compressGzip(content, config.gzipLevel)
				if err != nil {
					return fmt.Errorf("compress static file %q with gzip: %w", filepath, err)
				}
				file.addRepresentation(string(CompressionGzip), content, compressed)
			}
			if config.encodings[CompressionBrotli] {
				compressed, err := compressBrotli(content, config.brotliLevel)
				if err != nil {
					return fmt.Errorf("compress static file %q with Brotli: %w", filepath, err)
				}
				file.addRepresentation(string(CompressionBrotli), content, compressed)
			}
		}

		manifest.files[filepath] = file
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("prepare static filesystem: %w", err)
	}

	return &StaticFiles{manifest: manifest}, nil
}

func (file *staticFile) addRepresentation(encoding string, original, compressed []byte) {
	if len(compressed) >= len(original) {
		return
	}
	file.representations[encoding] = staticRepresentation{
		content: compressed,
		etag:    contentETag(compressed),
	}
}

func detectContentType(filepath string, content []byte) string {
	if contentType := mime.TypeByExtension(path.Ext(filepath)); contentType != "" {
		return contentType
	}
	return http.DetectContentType(content)
}

func isCompressible(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	if strings.HasPrefix(mediaType, "text/") {
		return true
	}

	switch mediaType {
	case "application/atom+xml",
		"application/javascript",
		"application/json",
		"application/ld+json",
		"application/manifest+json",
		"application/rss+xml",
		"application/wasm",
		"application/xhtml+xml",
		"application/xml",
		"image/svg+xml":
		return true
	default:
		return false
	}
}

func compressGzip(content []byte, level int) ([]byte, error) {
	var compressed bytes.Buffer
	writer, err := gzip.NewWriterLevel(&compressed, level)
	if err != nil {
		return nil, err
	}
	if _, err := writer.Write(content); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return compressed.Bytes(), nil
}

func compressBrotli(content []byte, level int) ([]byte, error) {
	var compressed bytes.Buffer
	writer := brotli.NewWriterLevel(&compressed, level)
	if _, err := writer.Write(content); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return compressed.Bytes(), nil
}
