package sse

import (
	"encoding/json"
	"net/http"
)

// Writer provides SSE response writing utilities
type Writer struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

// NewWriter creates a new SSE writer and sets appropriate headers
func NewWriter(w http.ResponseWriter) *Writer {
	flusher, _ := w.(http.Flusher)

	hdr := w.Header()
	hdr.Set("Content-Type", "text/event-stream")
	hdr.Set("Cache-Control", "no-cache, no-transform")
	hdr.Set("Connection", "keep-alive")
	hdr.Set("X-Accel-Buffering", "no")
	hdr.Del("Content-Encoding")

	return &Writer{w: w, flusher: flusher}
}

// Header returns the response header map to allow setting custom headers
// before writing the first chunk
func (sw *Writer) Header() http.Header {
	return sw.w.Header()
}

// WriteHeartbeat writes an SSE comment as a heartbeat/init signal
func (sw *Writer) WriteHeartbeat(msg string) error {
	if _, err := sw.w.Write([]byte(":" + msg + "\n\n")); err != nil {
		return err
	}
	sw.Flush()
	return nil
}

// WriteData writes a data event with JSON payload
func (sw *Writer) WriteData(data any) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return sw.WriteRaw(jsonData)
}

// WriteRaw writes raw bytes as an SSE data event
func (sw *Writer) WriteRaw(data []byte) error {
	if _, err := sw.w.Write([]byte("data: ")); err != nil {
		return err
	}
	if _, err := sw.w.Write(data); err != nil {
		return err
	}
	if _, err := sw.w.Write([]byte("\n\n")); err != nil {
		return err
	}
	sw.Flush()
	return nil
}

// WriteError writes an error event in a standard format
func (sw *Writer) WriteError(message string) error {
	return sw.WriteData(map[string]string{"error": message})
}

// WriteDone writes the [DONE] sentinel to signal stream end
func (sw *Writer) WriteDone() error {
	if _, err := sw.w.Write([]byte("data: [DONE]\n\n")); err != nil {
		return err
	}
	sw.Flush()
	return nil
}

// Flush flushes the response writer if it supports flushing
func (sw *Writer) Flush() {
	if sw.flusher != nil {
		sw.flusher.Flush()
	}
}
