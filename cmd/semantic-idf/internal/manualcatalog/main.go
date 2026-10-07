// Command manualcatalog updates the bundled metric reference from its Go registry.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func main() {
	output := flag.String("output", "cmd/semantic-idf/frontend/src/manual/metric-guides.json", "Bundled metric catalog destination (run from repository root).")
	flag.Parse()
	data, err := json.MarshalIndent(idf.MetricGuides(), "", "  ")
	if err == nil {
		err = os.WriteFile(*output, append(data, '\n'), 0644)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
