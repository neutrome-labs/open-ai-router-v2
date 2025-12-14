// Package sse provides Server-Sent Events reader and writer utilities.
package sse

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

// Event represents a single SSE event
type Event struct {
	Data    map[string]any
	RawData []byte // Raw JSON bytes for passthrough
	Error   error
	Done    bool
}

// Reader provides a streaming SSE parser
type Reader struct {
	scanner   *bufio.Scanner
	eventData bytes.Buffer
}

// NewReader creates a new SSE reader from an io.Reader
// bufSize is the initial buffer size, maxSize is the maximum event size
func NewReader(r io.Reader, bufSize, maxSize int) *Reader {
	scanner := bufio.NewScanner(r)
	buf := make([]byte, bufSize)
	scanner.Buffer(buf, maxSize)
	return &Reader{scanner: scanner}
}

// NewDefaultReader creates a reader with sensible defaults (64KB initial, 1MB max)
func NewDefaultReader(r io.Reader) *Reader {
	return NewReader(r, 64*1024, 1024*1024)
}

// ReadEvents returns a channel that emits SSE events
// The channel is closed when the stream ends or an error occurs
func (r *Reader) ReadEvents() <-chan Event {
	events := make(chan Event)

	go func() {
		defer close(events)

		for r.scanner.Scan() {
			line := r.scanner.Text()
			line = strings.TrimRight(line, "\r") // Handle Windows-style newlines

			// Comment/heartbeat line per SSE spec; ignore
			if strings.HasPrefix(line, ":") {
				continue
			}

			// Data field
			if strings.HasPrefix(line, "data:") {
				val := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				if r.eventData.Len() > 0 {
					r.eventData.WriteByte('\n')
				}
				r.eventData.WriteString(val)
				continue
			}

			// Other SSE fields (event, id, retry) - not used by OpenAI; ignore
			if strings.HasPrefix(line, "event:") ||
				strings.HasPrefix(line, "id:") ||
				strings.HasPrefix(line, "retry:") {
				continue
			}

			// Blank line indicates end of an event
			if strings.TrimSpace(line) == "" {
				if r.eventData.Len() > 0 {
					event := r.parseEvent(r.eventData.String())
					r.eventData.Reset()
					events <- event
					if event.Done || event.Error != nil {
						return
					}
				}
				continue
			}
			// Unknown line content; ignore
		}

		// Flush last event if stream ended without trailing blank line
		if r.eventData.Len() > 0 {
			events <- r.parseEvent(r.eventData.String())
		}

		if err := r.scanner.Err(); err != nil && err != io.EOF {
			events <- Event{Error: err}
		}
	}()

	return events
}

func (r *Reader) parseEvent(payload string) Event {
	if payload == "" {
		return Event{}
	}
	if payload == "[DONE]" {
		return Event{Done: true}
	}

	rawBytes := []byte(payload)
	var data map[string]any
	if err := json.Unmarshal(rawBytes, &data); err != nil {
		return Event{Error: err}
	}
	return Event{Data: data, RawData: rawBytes}
}
