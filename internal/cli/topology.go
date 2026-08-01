package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/internal/idf"
)

func cliTopology(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	fs := cliFlagSet("topology", stderr)
	level := fs.String("level", "zone", "Graph level: zone or boundary.")
	metric := fs.String("metric", "topology", "Metric: topology, area, ua, exposure, qa, or air.")
	scope := fs.String("scope", "building", "Scope: building, story, selection, or neighbors.")
	areaBasis := fs.String("area-basis", "effective", "Area basis: effective or physical.")
	format := fs.String("format", "json", "Output format: json, graphml, or dot.")
	output := ""
	fs.StringVar(&output, "output", "", "Output path. Defaults to stdout.")
	fs.StringVar(&output, "o", "", "Output path. Defaults to stdout.")
	storyIndex := fs.Int("story", 0, "Zero-based story index for story scope.")
	selection := fs.String("selection", "", "Stable entity ID for selection or neighbors scope.")
	neighborDepth := fs.Int("neighbor-depth", 1, "Neighbor traversal depth, from 1 to 3.")
	if err := fs.Parse(args); err != nil {
		return err
	}
	inputPath, err := singleInputPath(fs)
	if err != nil {
		return err
	}
	input, err := readCLIInput(inputPath, stdin)
	if err != nil {
		return err
	}

	index := idf.NewDocumentIndex(input.Doc)
	topology := idf.AnalyzeGeometryFromIndex(index).Topology
	options := idf.ThermalTopologyProjectionOptions{
		Level: *level, Metric: *metric, Scope: *scope, AreaBasis: *areaBasis,
		SelectedEntityID: *selection, NeighborDepth: *neighborDepth,
	}
	if strings.EqualFold(strings.TrimSpace(*scope), "story") {
		options.StoryIndex = storyIndex
	}

	var payload []byte
	switch normalizeCLIFormat(*format) {
	case "json":
		payload, err = idf.ExportThermalTopologyJSON(topology, options)
	case "graphml":
		payload, err = idf.ExportThermalTopologyGraphML(topology, options)
	case "dot":
		payload, err = idf.ExportThermalTopologyDOT(topology, options)
	default:
		return fmt.Errorf("unsupported topology format %q", *format)
	}
	if err != nil {
		return err
	}
	return writeCLITextOutput(output, payload, stdout)
}
