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
	hvac               HVACReport
	diagnostics        []Diagnostic
	profile            ProfileReport
	geometryBuildCount int
}

func newAnalysisSession(index *DocumentIndex) *analysisSession {
	if index == nil {
		index = NewDocumentIndex(Document{})
	}
	version := thermalEnergyPlusVersion(index.Doc)
	adapterVersion := fieldCatalogAdapter(version).AdapterVersion
	cacheKey := semanticStableHash(index.Doc.String()+"\x00"+adapterVersion+"\x00"+thermalTopologySchema, 32)
	return &analysisSession{index: index, cacheKey: cacheKey}
}

func (session *analysisSession) Geometry() GeometryReport {
	session.geometryOnce.Do(func() {
		session.geometryBuildCount++
		session.geometry = AnalyzeGeometryFromIndex(session.index)
	})
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
