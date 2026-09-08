package simulation

// This declaration contains reviewed source identities and model membership,
// never candidate values. The compiler performs ordinary SQL arithmetic without
// calling the production classifier, multiplier, allocation or quality helpers.
type epathRealSQLModel struct {
	Schema       string                     `json:"schema"`
	Surface      epathRealSQLSurfaceModel   `json:"surface"`
	Families     []epathRealSQLFamily       `json:"families"`
	Loads        []epathRealSQLLoad         `json:"loads"`
	Site         []epathRealSQLSite         `json:"site"`
	Availability []epathRealSQLAvailability `json:"availability"`
	Services     []epathRealSQLService      `json:"services"`
	Auxiliaries  []epathRealSQLAuxiliary    `json:"auxiliaries"`
	FanPools     []epathRealSQLFanPool      `json:"fanPools,omitempty"`
	DirectUses   []epathRealSQLDirectUse    `json:"directUses,omitempty"`
	Precision    epathRealSQLPrecision      `json:"precision"`
}

type epathRealSQLAlternative struct {
	Name string `json:"name"`
	Unit string `json:"unit"`
}
type epathRealSQLSelector struct {
	// Ordered alternatives are an explicit reviewed preference, not aliases
	// that may be summed. Components with different physics use separate terms.
	Alternatives []epathRealSQLAlternative `json:"alternatives"`
	Keys         []string                  `json:"keys"`
	IsMeter      bool                      `json:"isMeter"`
	AllowAbsent  bool                      `json:"allowAbsent,omitempty"`
}
type epathRealSQLTerm struct {
	Source epathRealSQLSelector `json:"source"`
	Sign   float64              `json:"sign"`
}
type epathRealSQLSurfaceModel struct {
	Source epathRealSQLSelector `json:"source"`
	Sign   float64              `json:"sign"`
	// Supported reviewed mapping is literal and ordered: internal mass/self,
	// interzone positive, ground -1, then outside0 class. Unknown rows fail.
	Mapping string `json:"mapping"`
}
type epathRealSQLFamily struct {
	ID        string             `json:"id"`
	Keys      []string           `json:"keys"`
	Category  string             `json:"category"`
	Component string             `json:"component"`
	Terms     []epathRealSQLTerm `json:"terms"`
	Subtract  []string           `json:"subtract,omitempty"`
	// pressure families participate in matching-service allocation; context
	// never becomes an input merely because its source reports a value.
	Role            string `json:"role"`
	BuildingVisible bool   `json:"buildingVisible"`
}
type epathRealSQLLoad struct {
	Service   string               `json:"service"`
	Component string               `json:"component"`
	Source    epathRealSQLSelector `json:"source"`
}
type epathRealSQLSite struct {
	ID       string               `json:"id"`
	EndUse   string               `json:"endUse,omitempty"`
	Carrier  string               `json:"carrier"`
	Facility bool                 `json:"facility,omitempty"`
	Source   epathRealSQLSelector `json:"source"`
}

// Explicit source identities, not values copied from a candidate. Only the
// declared exact Zone keys are owners; SQL supplies their effective multipliers.
type epathRealSQLDirectUse struct {
	EndUse  string               `json:"endUse"`
	Carrier string               `json:"carrier"`
	Source  epathRealSQLSelector `json:"source"`
}
type epathRealSQLAvailability struct {
	ID    string `json:"id"`
	Stage string `json:"stage"`
	// The names must also occur in the executed plan. Optional absent groups
	// are missing, not unrequested; their real request remains the denominator.
	RequestedNames []string                  `json:"requestedNames"`
	Observations   []epathRealSQLAlternative `json:"observations"`
	IsMeter        bool                      `json:"isMeter"`
}
type epathRealSQLService struct {
	Service           string   `json:"service"`
	SiteIDs           []string `json:"siteIds"`
	ServedZones       []string `json:"servedZones"`
	Basis             string   `json:"basis"`
	FallbackBasis     string   `json:"fallbackBasis"`
	RatioKind         string   `json:"ratioKind"`
	FallbackRatioKind string   `json:"fallbackRatioKind"`
	ReconciliationID  string   `json:"reconciliationId"`
}
type epathRealSQLAuxiliary struct {
	SiteID           string                `json:"siteId"`
	ServedZones      []string              `json:"servedZones"`
	Weight           string                `json:"weight"`
	WeightSource     *epathRealSQLSelector `json:"weightSource,omitempty"`
	AllocationMethod string                `json:"allocationMethod"`
	ReconciliationID string                `json:"reconciliationId"`
}
type epathRealSQLPrecision struct {
	DecimalPlaces int `json:"decimalPlaces"`
	// Counted rounding stages bound propagation, not duplicated execution.
	SourceStages       int `json:"sourceStages"`
	ContributionStages int `json:"contributionStages"`
}
