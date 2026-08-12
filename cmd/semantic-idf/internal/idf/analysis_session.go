package idf

import "sync"

type analysisSession struct {
	index              *DocumentIndex
	cacheKey           string
	geometryOnce       sync.Once
	hvacOnce           sync.Once
	diagnosticsOnce    sync.Once
	profileOnce        sync.Once
	geometry           GeometryReport
	geometryTiming     GeometryAnalysisTiming
	hvac               HVACReport
	diagnostics        []Diagnostic
	profile            ProfileReport
	geometryBuildCount int
}

func newAnalysisSession(index *DocumentIndex) *analysisSession {
	if index == nil {
		index = NewDocumentIndex(Document{})
	}
	return &analysisSession{index: index, cacheKey: newThermalGeometryCacheKey(index).String()}
}

func (session *analysisSession) Geometry() GeometryReport {
	return session.GeometryTimed(nil)
}

func (session *analysisSession) GeometryTimed(timer StageTimer) GeometryReport {
	session.geometryOnce.Do(func() {
		session.geometryBuildCount++
		session.geometry, session.geometryTiming = analyzeGeometryFromIndexCached(session.index)
	})
	if timer != nil {
		timer("geometry_transform", session.geometryTiming.GeometryTransform)
		timer("topology", session.geometryTiming.ThermalTopology)
	}
	return session.geometry
}

func (session *analysisSession) HVAC() HVACReport {
	session.hvacOnce.Do(func() {
		session.hvac = AnalyzeHVACFromIndex(session.index)
	})
	return session.hvac
}

func (session *analysisSession) Diagnostics() []Diagnostic {
	session.diagnosticsOnce.Do(func() {
		session.diagnostics = analyzeDiagnosticsWithReports(session.index.Doc, session.HVAC(), session.Geometry())
	})
	return append([]Diagnostic(nil), session.diagnostics...)
}

func (session *analysisSession) Profile() ProfileReport {
	session.profileOnce.Do(func() {
		session.profile = analyzeProfileWithGeometry(session.index.Doc, session.Geometry())
	})
	return session.profile
}
