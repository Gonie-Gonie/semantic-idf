package idf

// HVACOperatingProfile describes configured operating permission and control
// dependencies for one equipment occurrence. It does not assert simulated run
// status. The same component may have different profiles on different loops.
type HVACOperatingProfile struct {
	ComponentIndex          int                     `json:"componentIndex"`
	LoopID                  string                  `json:"loopId,omitempty"`
	AvailabilityLayers      []HVACAvailabilityLayer `json:"availabilityLayers"`
	FinalAvailability       HVACOperatingExpression `json:"finalAvailability"`
	Control                 HVACControlSequence     `json:"control"`
	AvailabilityDefinitions HVACControlSequence     `json:"availabilityDefinitions"`
	OnOff                   HVACControlSequence     `json:"onOff"`
	ParameterControl        HVACControlSequence     `json:"parameterControl"`
}

type HVACOperatingSource struct {
	ObjectIndex int    `json:"objectIndex"`
	ObjectType  string `json:"objectType"`
	ObjectName  string `json:"objectName"`
}

type HVACAvailabilityLayer struct {
	ID         string                  `json:"id"`
	Owner      HVACOperatingSource     `json:"owner"`
	Role       string                  `json:"role"`
	Expression HVACOperatingExpression `json:"expression"`
}

// Expressions have boolean (0/1/unknown) or manager-status semantics. Statuses
// are NoAction=0, ForceOff=1, CycleOn=2, CycleOnZoneFansOnly=3. air_managers uses
// EnergyPlus precedence; plant_managers keeps the first non-NoAction child.
// manager_gate converts status to permission, and fan_gate applies manager
// override to its second child (the fan's own availability schedule). target
// is central_fan or zone_fan where CycleOnZoneFansOnly needs disambiguation.
// Unknown is never interchangeable with a known zero or an omitted schedule.
type HVACOperatingExpression struct {
	Source       *HVACOperatingSource      `json:"source,omitempty"`
	Op           string                    `json:"op"`
	ScheduleName string                    `json:"scheduleName,omitempty"`
	Value        *float64                  `json:"value,omitempty"`
	Children     []HVACOperatingExpression `json:"children,omitempty"`
	Target       string                    `json:"target,omitempty"`
	Reason       string                    `json:"reason,omitempty"`
}

type HVACControlSequence struct {
	Nodes []HVACControlNode `json:"nodes"`
	Edges []HVACControlEdge `json:"edges"`
}

type HVACControlNode struct {
	ID         string                 `json:"id"`
	Kind       string                 `json:"kind"`
	Code       string                 `json:"code"`
	Source     HVACOperatingSource    `json:"source"`
	Parameters []HVACControlParameter `json:"parameters,omitempty"`
	Boundary   string                 `json:"boundary,omitempty"`
}

type HVACControlParameter struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	ObjectIndex int    `json:"objectIndex"`
	FieldIndex  int    `json:"fieldIndex"`
}

type HVACControlEdge struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Relation  string `json:"relation"`
	Condition string `json:"condition,omitempty"`
	Polarity  string `json:"polarity,omitempty"`
}
