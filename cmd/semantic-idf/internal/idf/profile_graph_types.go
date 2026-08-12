package idf

type ProfileGraphDeckState struct {
	ScopeType              string   `json:"scopeType"`
	SelectedGroupIDs       []string `json:"selectedGroupIds,omitempty"`
	SelectedZoneNames      []string `json:"selectedZoneNames,omitempty"`
	SelectedScheduleHashes []string `json:"selectedScheduleHashes,omitempty"`
	SelectedDimensions     []string `json:"selectedDimensions,omitempty"`
	MetricMode             string   `json:"metricMode"`
	TimeView               string   `json:"timeView"`
	CompareMode            string   `json:"compareMode"`
	ScaleMode              string   `json:"scaleMode"`
	TimeRange              []int    `json:"timeRange,omitempty"`
	PinnedSeriesIDs        []string `json:"pinnedSeriesIds,omitempty"`
}

type ProfileGraphDataset struct {
	DefaultDeck         ProfileGraphDeckState       `json:"defaultDeck"`
	MetricModes         []string                    `json:"metricModes"`
	TimeViews           []string                    `json:"timeViews"`
	CompareModes        []string                    `json:"compareModes"`
	ScaleModes          []string                    `json:"scaleModes"`
	Series              []ProfileGraphSeries        `json:"series"`
	ScheduleClusters    []ProfileScheduleCluster    `json:"scheduleClusters,omitempty"`
	Outliers            []ProfileOutlierHint        `json:"outliers,omitempty"`
	ParameterCandidates []ProfileParameterCandidate `json:"parameterCandidates,omitempty"`
}

type ProfileGraphSeries struct {
	ID                        string           `json:"id"`
	Label                     string           `json:"label"`
	ScopeType                 string           `json:"scopeType"`
	GroupID                   string           `json:"groupId,omitempty"`
	GroupName                 string           `json:"groupName,omitempty"`
	ZoneName                  string           `json:"zoneName,omitempty"`
	Dimension                 string           `json:"dimension"`
	DimensionLabel            string           `json:"dimensionLabel"`
	MetricID                  string           `json:"metricId"`
	MetricLabel               string           `json:"metricLabel"`
	Unit                      string           `json:"unit,omitempty"`
	DesignValue               float64          `json:"designValue"`
	DisplayValue              string           `json:"displayValue"`
	ScheduleName              string           `json:"scheduleName,omitempty"`
	ScheduleHash              string           `json:"scheduleHash,omitempty"`
	SchedulePattern           string           `json:"schedulePattern,omitempty"`
	Values                    []float64        `json:"values,omitempty"`
	DayMultiplierProfile      []float64        `json:"dayMultiplierProfile,omitempty"`
	WeekMultiplierProfile     []float64        `json:"weekMultiplierProfile,omitempty"`
	MonthMultiplierProfile    []float64        `json:"monthMultiplierProfile,omitempty"`
	AnnualMultiplierProfile   []float64        `json:"annualMultiplierProfile,omitempty"`
	DurationMultiplierProfile []float64        `json:"durationMultiplierProfile,omitempty"`
	RuleMultiplierProfile     []float64        `json:"ruleMultiplierProfile,omitempty"`
	SourceItemIDs             []string         `json:"sourceItemIds,omitempty"`
	SourceObjectIndexes       []int            `json:"sourceObjectIndexes,omitempty"`
	OperatingHours            float64          `json:"operatingHours"`
	EquivalentFullHours       float64          `json:"equivalentFullHours"`
	AnnualContribution        float64          `json:"annualContribution"`
	Peak                      float64          `json:"peak"`
	Status                    string           `json:"status"`
	Warnings                  []ProfileWarning `json:"warnings,omitempty"`
}

type ProfileScheduleCluster struct {
	ID                        string    `json:"id"`
	Label                     string    `json:"label"`
	ScheduleHash              string    `json:"scheduleHash"`
	Pattern                   string    `json:"pattern"`
	ScheduleNames             []string  `json:"scheduleNames"`
	ZoneNames                 []string  `json:"zoneNames"`
	Dimensions                []string  `json:"dimensions"`
	SeriesIDs                 []string  `json:"seriesIds"`
	FeatureVector             []float64 `json:"featureVector"`
	CentroidX                 float64   `json:"centroidX"`
	CentroidY                 float64   `json:"centroidY"`
	SameContentDifferentNames bool      `json:"sameContentDifferentNames"`
	SameNameDifferentContent  bool      `json:"sameNameDifferentContent"`
}

type ProfileOutlierHint struct {
	ID            string   `json:"id"`
	Severity      string   `json:"severity"`
	RuleID        string   `json:"ruleId"`
	Message       string   `json:"message"`
	ZoneName      string   `json:"zoneName,omitempty"`
	GroupID       string   `json:"groupId,omitempty"`
	Dimension     string   `json:"dimension,omitempty"`
	ScheduleName  string   `json:"scheduleName,omitempty"`
	ScheduleHash  string   `json:"scheduleHash,omitempty"`
	Value         float64  `json:"value,omitempty"`
	Median        float64  `json:"median,omitempty"`
	MAD           float64  `json:"mad,omitempty"`
	Score         float64  `json:"score,omitempty"`
	SourceItemIDs []string `json:"sourceItemIds,omitempty"`
	SeriesIDs     []string `json:"seriesIds,omitempty"`
	CandidateID   string   `json:"candidateId,omitempty"`
}

type ProfileParameterCandidate struct {
	ID            string               `json:"id"`
	Label         string               `json:"label"`
	Dimension     string               `json:"dimension"`
	MetricID      string               `json:"metricId"`
	Reason        string               `json:"reason"`
	Severity      string               `json:"severity"`
	ZoneNames     []string             `json:"zoneNames"`
	SourceItemIDs []string             `json:"sourceItemIds"`
	RuleIDs       []string             `json:"ruleIds"`
	CurrentMin    float64              `json:"currentMin"`
	CurrentMedian float64              `json:"currentMedian"`
	CurrentMax    float64              `json:"currentMax"`
	ImpactScore   float64              `json:"impactScore"`
	ApplyRequest  *ProfileApplyRequest `json:"applyRequest,omitempty"`
}
