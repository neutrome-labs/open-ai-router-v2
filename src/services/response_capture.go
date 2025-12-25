package services

import "net/http"

// ResponseCaptureWriter captures response instead of writing to HTTP
type ResponseCaptureWriter struct {
	Response []byte
	Headers  http.Header
}

func (w *ResponseCaptureWriter) Header() http.Header {
	if w.Headers == nil {
		w.Headers = make(http.Header)
	}
	return w.Headers
}

func (w *ResponseCaptureWriter) Write(data []byte) (int, error) {
	w.Response = data
	return len(data), nil
}

func (w *ResponseCaptureWriter) WriteHeader(statusCode int) {
	// Ignore for capture
}
