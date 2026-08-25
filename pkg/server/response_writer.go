package server

import "net/http"

type statusResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (writer *statusResponseWriter) WriteHeader(statusCode int) {
	if writer.statusCode != 0 {
		return
	}
	writer.statusCode = statusCode
	writer.ResponseWriter.WriteHeader(statusCode)
}

func (writer *statusResponseWriter) Write(content []byte) (int, error) {
	if writer.statusCode == 0 {
		writer.WriteHeader(http.StatusOK)
	}
	return writer.ResponseWriter.Write(content)
}

func (writer *statusResponseWriter) StatusCode() int {
	if writer.statusCode == 0 {
		return http.StatusOK
	}
	return writer.statusCode
}

// Unwrap allows http.ResponseController to reach the underlying writer.
func (writer *statusResponseWriter) Unwrap() http.ResponseWriter {
	return writer.ResponseWriter
}

func responseStatusCode(writer http.ResponseWriter) int {
	if statusWriter, ok := writer.(interface{ StatusCode() int }); ok {
		return statusWriter.StatusCode()
	}
	return http.StatusOK
}
