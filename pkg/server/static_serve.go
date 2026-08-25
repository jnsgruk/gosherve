package server

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

var errNotAcceptable = errors.New("no acceptable content encoding")

type selectedRepresentation struct {
	content  []byte
	encoding string
	etag     string
}

func servePreparedFile(w http.ResponseWriter, r *http.Request, manifest *staticManifest, file *staticFile) error {
	if strings.HasSuffix(r.URL.Path, "/index.html") {
		w.Header().Set("ETag", file.etag)
		localRedirect(w, r, "./")
		return nil
	}

	representation, err := selectRepresentation(file, r.Header.Values("Accept-Encoding"))
	if err != nil {
		return err
	}
	setRepresentationHeaders(w.Header(), file, representation)
	return servePreparedRepresentation(w, r, manifest, file, representation)
}

func servePreparedError(w http.ResponseWriter, r *http.Request, manifest *staticManifest, file *staticFile, status int) error {
	representation, err := selectRepresentation(file, r.Header.Values("Accept-Encoding"))
	if err != nil {
		return err
	}
	setRepresentationHeaders(w.Header(), file, representation)

	request := r.Clone(r.Context())
	request.Header.Del("Range")
	request.Header.Del("If-Range")
	statusWriter := &overrideStatusResponseWriter{ResponseWriter: w, statusCode: status}
	if err := servePreparedRepresentation(statusWriter, request, manifest, file, representation); err != nil {
		return err
	}
	statusWriter.ensureStatus()
	return nil
}

func servePreparedRepresentation(w http.ResponseWriter, r *http.Request, manifest *staticManifest, file *staticFile, representation selectedRepresentation) error {
	if representation.encoding != identityEncoding {
		http.ServeContent(w, r, file.path, file.modTime, bytes.NewReader(representation.content))
		return nil
	}

	content, err := manifest.source.Open(file.path)
	if err != nil {
		return fmt.Errorf("open prepared file %q: %w", file.path, err)
	}
	defer content.Close()

	seeker, ok := content.(io.ReadSeeker)
	if !ok {
		return fmt.Errorf("prepared file %q does not implement io.ReadSeeker", file.path)
	}
	http.ServeContent(w, r, file.path, file.modTime, seeker)
	return nil
}

func localRedirect(w http.ResponseWriter, r *http.Request, location string) {
	if r.URL.RawQuery != "" {
		location += "?" + r.URL.RawQuery
	}
	w.Header().Set("Location", location)
	w.WriteHeader(http.StatusMovedPermanently)
}

type overrideStatusResponseWriter struct {
	http.ResponseWriter
	statusCode  int
	wroteHeader bool
}

func (writer *overrideStatusResponseWriter) WriteHeader(statusCode int) {
	if writer.wroteHeader {
		return
	}
	writer.wroteHeader = true
	if statusCode == http.StatusOK {
		statusCode = writer.statusCode
	}
	writer.ResponseWriter.WriteHeader(statusCode)
}

func (writer *overrideStatusResponseWriter) Write(content []byte) (int, error) {
	if !writer.wroteHeader {
		writer.WriteHeader(http.StatusOK)
	}
	return writer.ResponseWriter.Write(content)
}

func (writer *overrideStatusResponseWriter) ensureStatus() {
	if !writer.wroteHeader {
		writer.WriteHeader(http.StatusOK)
	}
}

func setRepresentationHeaders(header http.Header, file *staticFile, representation selectedRepresentation) {
	header.Set("Content-Type", file.contentType)
	header.Set("ETag", representation.etag)
	if len(file.representations) > 0 {
		addVary(header, "Accept-Encoding")
	}
	if representation.encoding != identityEncoding {
		header.Set("Content-Encoding", representation.encoding)
	}
}

func addVary(header http.Header, value string) {
	for _, existing := range header.Values("Vary") {
		for _, token := range strings.Split(existing, ",") {
			if strings.EqualFold(strings.TrimSpace(token), value) {
				return
			}
		}
	}
	header.Add("Vary", value)
}

func selectRepresentation(file *staticFile, acceptEncoding []string) (selectedRepresentation, error) {
	available := map[string]selectedRepresentation{
		identityEncoding: {
			encoding: identityEncoding,
			etag:     file.etag,
		},
	}
	for encoding, representation := range file.representations {
		available[encoding] = selectedRepresentation{
			content:  representation.content,
			encoding: encoding,
			etag:     representation.etag,
		}
	}

	encoding, ok := negotiateContentEncoding(acceptEncoding, available)
	if !ok {
		return selectedRepresentation{}, errNotAcceptable
	}
	return available[encoding], nil
}

func negotiateContentEncoding(acceptEncoding []string, available map[string]selectedRepresentation) (string, bool) {
	header := strings.TrimSpace(strings.Join(acceptEncoding, ","))
	if header == "" {
		_, ok := available[identityEncoding]
		return identityEncoding, ok
	}

	quality := map[string]float64{}
	for _, value := range strings.Split(header, ",") {
		parts := strings.Split(value, ";")
		encoding := strings.ToLower(strings.TrimSpace(parts[0]))
		if encoding == "" {
			continue
		}
		q := 1.0
		for _, parameter := range parts[1:] {
			name, raw, found := strings.Cut(strings.TrimSpace(parameter), "=")
			if !found || !strings.EqualFold(strings.TrimSpace(name), "q") {
				continue
			}
			parsed, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
			if err != nil || parsed < 0 || parsed > 1 {
				q = 0
			} else {
				q = parsed
			}
		}
		if previous, exists := quality[encoding]; !exists || q > previous {
			quality[encoding] = q
		}
	}

	bestEncoding := ""
	bestQuality := -1.0
	for _, encoding := range []string{string(CompressionBrotli), string(CompressionGzip), identityEncoding} {
		if _, ok := available[encoding]; !ok {
			continue
		}

		q, explicit := quality[encoding]
		if !explicit {
			if encoding == identityEncoding {
				q = 1
				if wildcard, exists := quality["*"]; exists && wildcard == 0 {
					q = 0
				}
			} else if wildcard, exists := quality["*"]; exists {
				q = wildcard
			} else {
				q = 0
			}
		}

		if q > 0 && q > bestQuality {
			bestEncoding = encoding
			bestQuality = q
		}
	}

	return bestEncoding, bestEncoding != ""
}
