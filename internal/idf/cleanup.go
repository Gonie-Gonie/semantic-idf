package idf

import (
	"fmt"
	"sort"
	"strings"
)

const (
	CleanupRuleUnusedSchedules         = "remove_unused_schedules"
	CleanupRuleUnusedEnvelopeResources = "remove_unused_envelope_resources"
	CleanupRuleUnusedCurvesTables      = "remove_unused_curves_tables"
	CleanupRuleDuplicateOutputVars     = "remove_duplicate_output_variables"
	CleanupRuleCompactFormatting       = "compact_formatting"
	CleanupRuleCommentsOnly            = "remove_comments_only"
)

type CleanupRule struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Group       string `json:"group,omitempty"`
	Available   bool   `json:"available"`
	Default     bool   `json:"default"`
	Future      bool   `json:"future,omitempty"`
}

type CleanupCandidate struct {
	Key          string   `json:"key"`
	RuleID       string   `json:"ruleId"`
	ObjectIndex  int      `json:"objectIndex"`
	ObjectType   string   `json:"objectType"`
	ObjectName   string   `json:"objectName,omitempty"`
	Reason       string   `json:"reason"`
	RelatedCodes []string `json:"relatedCodes,omitempty"`
	Risk         string   `json:"risk,omitempty"`
	Source       string   `json:"source,omitempty"`
}

type CleanupScan struct {
	Rules      []CleanupRule      `json:"rules"`
	Candidates []CleanupCandidate `json:"candidates"`
}

type CleanupPreview struct {
	RemovedCandidates []CleanupCandidate `json:"removedCandidates"`
	RemovedCount      int                `json:"removedCount"`
}

func ScanCleanup(doc Document) CleanupScan {
	candidates := cleanupCandidates(doc)
	return CleanupScan{
		Rules:      cleanupRules(candidates),
		Candidates: candidates,
	}
}

func PreviewCleanup(doc Document, ruleIDs []string, excludedCandidateKeys ...[]string) CleanupPreview {
	selected := selectedCleanupRules(ruleIDs)
	excluded := selectedCleanupCandidateKeys(excludedCandidateKeys)
	var removed []CleanupCandidate
	for _, candidate := range cleanupCandidates(doc) {
		if selected[candidate.RuleID] && !excluded[candidate.Key] {
			removed = append(removed, candidate)
		}
	}
	return CleanupPreview{
		RemovedCandidates: removed,
		RemovedCount:      len(removed),
	}
}

func ApplyCleanup(doc Document, ruleIDs []string, excludedCandidateKeys ...[]string) (Document, CleanupPreview) {
	preview := PreviewCleanup(doc, ruleIDs, excludedCandidateKeys...)
	if len(preview.RemovedCandidates) == 0 {
		return doc.clone(), preview
	}
	removeIndexes := map[int]bool{}
	for _, candidate := range preview.RemovedCandidates {
		removeIndexes[candidate.ObjectIndex] = true
	}

	var objects []Object
	for _, obj := range doc.Objects {
		if removeIndexes[obj.Index] {
			continue
		}
		obj.Index = len(objects)
		objects = append(objects, obj)
	}
	return Document{Objects: objects}, preview
}

func CleanupCompacts(ruleIDs []string) bool {
	return selectedCleanupRules(ruleIDs)[CleanupRuleCompactFormatting]
}

func cleanupRules(candidates []CleanupCandidate) []CleanupRule {
	counts := map[string]int{}
	for _, candidate := range candidates {
		counts[candidate.RuleID]++
	}
	rules := []CleanupRule{
		{
			ID:          CleanupRuleUnusedSchedules,
			Name:        "Remove unused schedules",
			Description: "Remove Schedule:* objects that are not referenced by other fields.",
			Group:       "safe",
			Available:   counts[CleanupRuleUnusedSchedules] > 0,
			Default:     counts[CleanupRuleUnusedSchedules] > 0,
		},
		{
			ID:          CleanupRuleUnusedEnvelopeResources,
			Name:        "Remove unused materials/constructions",
			Description: "Remove unreferenced material, window material, and construction resources.",
			Group:       "review",
			Available:   counts[CleanupRuleUnusedEnvelopeResources] > 0,
			Default:     false,
		},
		{
			ID:          CleanupRuleUnusedCurvesTables,
			Name:        "Remove unused curves/tables",
			Description: "Remove unreferenced performance curves and tables.",
			Group:       "review",
			Available:   counts[CleanupRuleUnusedCurvesTables] > 0,
			Default:     false,
		},
		{
			ID:          CleanupRuleDuplicateOutputVars,
			Name:        "Remove duplicate Output objects",
			Description: "Keep the first Output:* or OutputControl:Table:Style object for each identical field signature.",
			Group:       "output",
			Available:   counts[CleanupRuleDuplicateOutputVars] > 0,
			Default:     counts[CleanupRuleDuplicateOutputVars] > 0,
		},
		{
			ID:          CleanupRuleCommentsOnly,
			Name:        "Remove comments only",
			Description: "Reserved for a future comment-only cleanup pass.",
			Group:       "experimental",
			Available:   false,
			Default:     false,
			Future:      true,
		},
		{
			ID:          CleanupRuleCompactFormatting,
			Name:        "Compact formatting",
			Description: "Rewrite IDF object spacing using the app formatter without removing extra objects.",
			Group:       "formatting",
			Available:   true,
			Default:     false,
		},
	}
	return rules
}

func cleanupCandidates(doc Document) []CleanupCandidate {
	var candidates []CleanupCandidate
	for _, obj := range FindUnusedObjects(doc) {
		ruleID := cleanupRuleForUnusedObject(obj.Type)
		if ruleID == "" {
			continue
		}
		candidates = append(candidates, CleanupCandidate{
			Key:          cleanupCandidateKey(ruleID, obj.Index),
			RuleID:       ruleID,
			ObjectIndex:  obj.Index,
			ObjectType:   obj.Type,
			ObjectName:   obj.Name,
			Reason:       fmt.Sprintf("%s %q is not referenced.", obj.Type, obj.Name),
			RelatedCodes: []string{"orphan_object"},
			Risk:         cleanupRiskForRule(ruleID),
			Source:       "analyzer",
		})
	}
	candidates = append(candidates, duplicateOutputCandidates(doc)...)
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].RuleID != candidates[j].RuleID {
			return candidates[i].RuleID < candidates[j].RuleID
		}
		return candidates[i].ObjectIndex < candidates[j].ObjectIndex
	})
	return candidates
}

func cleanupRuleForUnusedObject(objectType string) string {
	lower := strings.ToLower(objectType)
	switch {
	case strings.HasPrefix(lower, "schedule:"):
		return CleanupRuleUnusedSchedules
	case strings.HasPrefix(lower, "material") ||
		strings.HasPrefix(lower, "windowmaterial") ||
		strings.HasPrefix(lower, "construction"):
		return CleanupRuleUnusedEnvelopeResources
	case strings.HasPrefix(lower, "curve:") ||
		strings.HasPrefix(lower, "table:"):
		return CleanupRuleUnusedCurvesTables
	default:
		return ""
	}
}

func duplicateOutputCandidates(doc Document) []CleanupCandidate {
	seen := map[string]Object{}
	var candidates []CleanupCandidate
	for _, obj := range doc.Objects {
		if !cleanupDuplicateOutputType(obj.Type) {
			continue
		}
		signature := outputObjectSignature(obj.Type, outputFieldValues(obj))
		if first, ok := seen[signature]; ok {
			candidates = append(candidates, CleanupCandidate{
				Key:          cleanupCandidateKey(CleanupRuleDuplicateOutputVars, obj.Index),
				RuleID:       CleanupRuleDuplicateOutputVars,
				ObjectIndex:  obj.Index,
				ObjectType:   obj.Type,
				ObjectName:   objectName(obj),
				Reason:       fmt.Sprintf("Duplicate %s already defined by object #%d.", obj.Type, first.Index+1),
				RelatedCodes: []string{"duplicate_output_request"},
				Risk:         cleanupRiskForRule(CleanupRuleDuplicateOutputVars),
				Source:       "output",
			})
			continue
		}
		seen[signature] = obj
	}
	return candidates
}

func cleanupDuplicateOutputType(objectType string) bool {
	lower := strings.ToLower(strings.TrimSpace(objectType))
	switch lower {
	case "output:variable",
		"output:meter",
		"output:meter:meterfileonly",
		"output:meter:cumulative",
		"output:meter:cumulative:meterfileonly",
		"output:sqlite",
		"output:json",
		"outputcontrol:table:style":
		return true
	default:
		return strings.HasPrefix(lower, "output:table:")
	}
}

func cleanupRiskForRule(ruleID string) string {
	switch ruleID {
	case CleanupRuleUnusedSchedules, CleanupRuleDuplicateOutputVars, CleanupRuleCompactFormatting:
		return "safe"
	default:
		return "review"
	}
}

func selectedCleanupRules(ruleIDs []string) map[string]bool {
	out := map[string]bool{}
	for _, ruleID := range ruleIDs {
		ruleID = strings.TrimSpace(ruleID)
		if ruleID != "" {
			out[ruleID] = true
		}
	}
	return out
}

func selectedCleanupCandidateKeys(groups [][]string) map[string]bool {
	out := map[string]bool{}
	for _, group := range groups {
		for _, key := range group {
			key = strings.TrimSpace(key)
			if key != "" {
				out[key] = true
			}
		}
	}
	return out
}

func cleanupCandidateKey(ruleID string, objectIndex int) string {
	return fmt.Sprintf("%s:%d", ruleID, objectIndex)
}
