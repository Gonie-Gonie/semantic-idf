package simulation

import (
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Gonie-Gonie/idf-analyzer/internal/idf"
)

type OutputDiscoveryRequest struct {
	Text            string                    `json:"text,omitempty"`
	OutputDirectory string                    `json:"outputDirectory,omitempty"`
	SQLPath         string                    `json:"sqlPath,omitempty"`
	RDDPath         string                    `json:"rddPath,omitempty"`
	MDDPath         string                    `json:"mddPath,omitempty"`
	PurposeRequest  *SimulationPurposeRequest `json:"purposeRequest,omitempty"`
}

type OutputDiscoveryResult struct {
	Items   []OutputDiscoveryItem `json:"items"`
	Sources []string              `json:"sources,omitempty"`
	Counts  map[string]int        `json:"counts,omitempty"`
}

type OutputDiscoveryItem struct {
	ObjectType         string                `json:"objectType"`
	KeyValue           string                `json:"keyValue,omitempty"`
	Name               string                `json:"name"`
	Units              string                `json:"units,omitempty"`
	ReportingFrequency string                `json:"reportingFrequency,omitempty"`
	Source             string                `json:"source"`
	Status             string                `json:"status"`
	PurposeIDs         []SimulationPurposeID `json:"purposeIds,omitempty"`
}

func DiscoverAvailableOutputs(request OutputDiscoveryRequest) (OutputDiscoveryResult, error) {
	result := OutputDiscoveryResult{Counts: map[string]int{}}
	collector := outputDiscoveryCollector{items: map[string]OutputDiscoveryItem{}}
	for _, path := range discoverySQLPaths(request) {
		items, err := discoverOutputsFromSQL(path)
		if err != nil {
			continue
		}
		for _, item := range items {
			collector.add(item)
		}
		if len(items) > 0 {
			result.Sources = append(result.Sources, filepath.Base(path))
		}
	}
	for _, path := range discoveryRDDPaths(request) {
		items, err := discoverOutputsFromRDD(path)
		if err != nil {
			continue
		}
		for _, item := range items {
			collector.add(item)
		}
		if len(items) > 0 {
			result.Sources = append(result.Sources, filepath.Base(path))
		}
	}
	for _, path := range discoveryMDDPaths(request) {
		items, err := discoverOutputsFromMDD(path)
		if err != nil {
			continue
		}
		for _, item := range items {
			collector.add(item)
		}
		if len(items) > 0 {
			result.Sources = append(result.Sources, filepath.Base(path))
		}
	}
	if strings.TrimSpace(request.Text) != "" && request.PurposeRequest != nil {
		doc, err := idf.Parse(request.Text)
		if err == nil {
			plan := BuildPurposeRunPlan(doc, NormalizeSimulationPurposeRequest(request.PurposeRequest))
			for _, object := range plan.OutputObjects {
				if !purposeObjectIsSeries(object.ObjectType) {
					continue
				}
				status := "fallback"
				if collector.has(object.ObjectType, object.KeyValue, object.VariableName) {
					status = "available"
				}
				collector.add(OutputDiscoveryItem{
					ObjectType:         object.ObjectType,
					KeyValue:           object.KeyValue,
					Name:               object.VariableName,
					ReportingFrequency: object.ReportingFrequency,
					Source:             "purpose_plan",
					Status:             status,
					PurposeIDs:         object.PurposeIDs,
				})
			}
		}
	}
	result.Items = collector.sorted()
	for _, item := range result.Items {
		result.Counts[item.Status]++
		if item.Source != "" {
			result.Counts["source:"+item.Source]++
		}
	}
	sort.Strings(result.Sources)
	result.Sources = normalizePurposeStrings(result.Sources)
	return result, nil
}

type outputDiscoveryCollector struct {
	items map[string]OutputDiscoveryItem
}

func (collector *outputDiscoveryCollector) add(item OutputDiscoveryItem) {
	item.ObjectType = strings.TrimSpace(item.ObjectType)
	item.KeyValue = strings.TrimSpace(item.KeyValue)
	item.Name = strings.TrimSpace(item.Name)
	item.Units = strings.TrimSpace(item.Units)
	item.ReportingFrequency = canonicalPurposeFrequency(item.ReportingFrequency)
	item.Source = strings.TrimSpace(item.Source)
	item.Status = strings.TrimSpace(item.Status)
	if item.Status == "" {
		item.Status = "available"
	}
	if item.ObjectType == "" || item.Name == "" {
		return
	}
	key := outputDiscoveryKey(item.ObjectType, item.KeyValue, item.Name)
	existing, ok := collector.items[key]
	if ok {
		existing.Source = mergeDiscoveryToken(existing.Source, item.Source)
		existing.Status = mergeDiscoveryStatus(existing.Status, item.Status)
		existing.PurposeIDs = normalizePurposeIDs(append(existing.PurposeIDs, item.PurposeIDs...))
		if existing.Units == "" {
			existing.Units = item.Units
		}
		if existing.ReportingFrequency == "" {
			existing.ReportingFrequency = item.ReportingFrequency
		}
		collector.items[key] = existing
		return
	}
	collector.items[key] = item
}

func (collector outputDiscoveryCollector) has(objectType string, keyValue string, name string) bool {
	if collector.items[outputDiscoveryKey(objectType, keyValue, name)].Name != "" {
		return true
	}
	return collector.items[outputDiscoveryKey(objectType, "*", name)].Name != ""
}

func (collector outputDiscoveryCollector) sorted() []OutputDiscoveryItem {
	out := make([]OutputDiscoveryItem, 0, len(collector.items))
	for _, item := range collector.items {
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ObjectType != out[j].ObjectType {
			return strings.ToLower(out[i].ObjectType) < strings.ToLower(out[j].ObjectType)
		}
		if out[i].Name != out[j].Name {
			return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
		}
		return strings.ToLower(out[i].KeyValue) < strings.ToLower(out[j].KeyValue)
	})
	return out
}

func discoverySQLPaths(request OutputDiscoveryRequest) []string {
	return discoveryPaths(request.SQLPath, request.OutputDirectory, "eplusout.sql", ".sql")
}

func discoveryRDDPaths(request OutputDiscoveryRequest) []string {
	return discoveryPaths(request.RDDPath, request.OutputDirectory, "eplusout.rdd", ".rdd")
}

func discoveryMDDPaths(request OutputDiscoveryRequest) []string {
	return discoveryPaths(request.MDDPath, request.OutputDirectory, "eplusout.mdd", ".mdd")
}

func discoveryPaths(explicit string, outputDirectory string, defaultName string, extension string) []string {
	paths := []string{}
	if strings.TrimSpace(explicit) != "" {
		paths = append(paths, explicit)
	}
	if strings.TrimSpace(outputDirectory) != "" {
		paths = append(paths, filepath.Join(outputDirectory, defaultName))
		entries, err := os.ReadDir(outputDirectory)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), extension) {
					continue
				}
				paths = append(paths, filepath.Join(outputDirectory, entry.Name()))
			}
		}
	}
	return normalizePurposeStrings(paths)
}

func discoverOutputsFromSQL(path string) ([]OutputDiscoveryItem, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	exists, err := sqlTableExists(db, "ReportDataDictionary")
	if err != nil || !exists {
		return nil, err
	}
	rows, err := db.Query(`
SELECT DISTINCT COALESCE(KeyValue, ''), COALESCE(Name, ''), COALESCE(Units, '')
FROM ReportDataDictionary
WHERE TRIM(COALESCE(Name, '')) <> ''
ORDER BY Name, KeyValue`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []OutputDiscoveryItem
	for rows.Next() {
		var keyValue, name, units string
		if err := rows.Scan(&keyValue, &name, &units); err != nil {
			continue
		}
		items = append(items, OutputDiscoveryItem{
			ObjectType: "Output:Variable",
			KeyValue:   keyValue,
			Name:       name,
			Units:      units,
			Source:     "sql",
			Status:     "available",
		})
	}
	return items, rows.Err()
}

func discoverOutputsFromRDD(path string) ([]OutputDiscoveryItem, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	items := []OutputDiscoveryItem{}
	for _, line := range strings.Split(string(content), "\n") {
		parts := dictionaryLineParts(line)
		if len(parts) < 2 {
			continue
		}
		name, units := splitDictionaryNameUnits(parts[len(parts)-1])
		keyValue := "*"
		if len(parts) >= 3 {
			keyValue = parts[0]
		}
		items = append(items, OutputDiscoveryItem{
			ObjectType: "Output:Variable",
			KeyValue:   keyValue,
			Name:       name,
			Units:      units,
			Source:     "rdd",
			Status:     "available",
		})
	}
	return items, nil
}

func discoverOutputsFromMDD(path string) ([]OutputDiscoveryItem, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	items := []OutputDiscoveryItem{}
	for _, line := range strings.Split(string(content), "\n") {
		parts := dictionaryLineParts(line)
		if len(parts) == 0 {
			continue
		}
		name, units := splitDictionaryNameUnits(parts[len(parts)-1])
		items = append(items, OutputDiscoveryItem{
			ObjectType: "Output:Meter",
			Name:       name,
			Units:      units,
			Source:     "mdd",
			Status:     "available",
		})
	}
	return items, nil
}

func dictionaryLineParts(line string) []string {
	line = strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
	if line == "" || strings.HasPrefix(line, "!") || strings.HasPrefix(strings.ToLower(line), "program version") {
		return nil
	}
	raw := strings.Split(line, ",")
	parts := make([]string, 0, len(raw))
	for _, part := range raw {
		part = strings.TrimSpace(part)
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

func splitDictionaryNameUnits(value string) (string, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ""
	}
	start := strings.LastIndex(value, "[")
	end := strings.LastIndex(value, "]")
	if start >= 0 && end > start {
		return strings.TrimSpace(value[:start]), strings.TrimSpace(value[start+1 : end])
	}
	return value, ""
}

func outputDiscoveryKey(objectType string, keyValue string, name string) string {
	return normalizePurposeToken(objectType) + "|" + normalizePurposeToken(keyValue) + "|" + normalizePurposeToken(name)
}

func mergeDiscoveryToken(left string, right string) string {
	if strings.TrimSpace(left) == "" {
		return strings.TrimSpace(right)
	}
	if strings.TrimSpace(right) == "" || strings.Contains("|"+left+"|", "|"+right+"|") {
		return left
	}
	return left + "|" + right
}

func mergeDiscoveryStatus(left string, right string) string {
	if left == "available" || right == "available" {
		return "available"
	}
	if left == "alias" || right == "alias" {
		return "alias"
	}
	if left == "fallback" || right == "fallback" {
		return "fallback"
	}
	return strings.TrimSpace(left + " " + right)
}
