package main

import (
	"io"
	"net/http"
	"strconv"
	"strings"
)

const compactSimulationResultMediaType = "application/vnd.semantic-idf.simulation-transfer+json"

func acceptsCompactSimulationResult(r *http.Request) bool {
	for _, mediaType := range strings.Split(r.Header.Get("Accept"), ",") {
		if strings.TrimSpace(mediaType) == compactSimulationResultMediaType {
			return true
		}
	}
	return false
}

// Send the existing immutable JSON snapshot as a response body. Passing it to
// json.Encoder would scan and copy the full result again; returning it through
// the desktop callback would additionally escape it into a JavaScript string.
func writeSimulationResultBytes(w http.ResponseWriter, payload []byte) error {
	if payload == nil {
		payload = []byte("null")
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
	w.Header().Set("Cache-Control", "no-store")
	n, err := w.Write(payload)
	if err == nil && n != len(payload) {
		return io.ErrShortWrite
	}
	return err
}
