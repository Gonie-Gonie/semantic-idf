package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/simulation"
)

// LoadEnergyPath reads an existing result. It does not run EnergyPlus, alter the
// active document, or replace the desktop's current simulation result cache.
func (a *App) LoadEnergyPath(request simulation.EnergyPathProjectionRequest) (simulation.EnergyPathProjection, error) {
	return simulation.LoadEnergyPathProjection(request)
}

func serveEnergyPathProjection(w http.ResponseWriter, r *http.Request, app *App) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request struct {
		simulation.EnergyPathProjectionRequest
		Format       string `json:"format"`
		IncludeTrace bool   `json:"includeTrace"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		http.Error(w, "expected exactly one JSON request object", http.StatusBadRequest)
		return
	}
	format := strings.ToLower(strings.TrimSpace(request.Format))
	if format == "" {
		format = "json"
	}
	if format != "json" && format != "csv" {
		http.Error(w, fmt.Sprintf("unsupported energy-path format %q; use json or csv", request.Format), http.StatusBadRequest)
		return
	}
	projection, err := app.LoadEnergyPath(request.EnergyPathProjectionRequest)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if format == "csv" {
		payload, err := simulation.EnergyPathProjectionCSV(projection, request.IncludeTrace)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		_, _ = io.WriteString(w, payload)
		return
	}
	// Trace is a CSV presentation option. JSON always preserves the same
	// canonical v2 payload, including provenance, as the desktop/CLI builder.
	payload, err := json.Marshal(projection)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(payload)
}
