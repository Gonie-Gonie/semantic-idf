package simulation

// Literal original-model/source declarations, never candidate graph quantities.
// Local standalone consumers retain the independently validated DirectHVAC
// source registry; Shared contains only additional native shared constituents.
type epathRealSQLHVACConsumptionPool struct {
	SiteID string                         `json:"siteId"`
	Shared []epathRealSQLHVACSharedMember `json:"shared"`
}

type epathRealSQLHVACSharedMember struct {
	ID            string               `json:"id"`
	ObjectType    string               `json:"objectType"`
	ObjectName    string               `json:"objectName"`
	PlantLoopName string               `json:"plantLoopName"`
	ServedZones   []string             `json:"servedZones"`
	Source        epathRealSQLSelector `json:"source"`
}

type epathSQLHVACSharedSourceIdentity struct {
	Member     epathRealSQLHVACSharedMember
	Source     epathRealSQLSource
	Companions []epathRealSQLSource
	Precision  epathRealSQLPrecision
}

type epathSQLHVACConsumptionPoolFrame struct {
	Declaration epathRealSQLHVACConsumptionPool
	Shared      []epathSQLHVACSharedSourceIdentity
}
