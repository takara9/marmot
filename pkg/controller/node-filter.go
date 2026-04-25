package controller

import (
	"strings"

	"github.com/takara9/marmot/api"
)

// evaluateNodeAssignment returns process decision, assigned node, and reason.
// Reasons:
// - controller_node_empty
// - metadata_missing
// - assigned_node_missing
// - assigned_node_empty
// - assigned_node_match
// - assigned_node_mismatch
func evaluateNodeAssignment(metadata *api.Metadata, nodeName string) (bool, string, string) {
	if strings.TrimSpace(nodeName) == "" {
		return true, "", "controller_node_empty"
	}
	if metadata == nil || metadata.NodeName == nil {
		if metadata == nil {
			return true, "", "metadata_missing"
		}
		return true, "", "assigned_node_missing"
	}
	assigned := strings.TrimSpace(*metadata.NodeName)
	if assigned == "" {
		return true, "", "assigned_node_empty"
	}
	if assigned == nodeName {
		return true, assigned, "assigned_node_match"
	}
	return false, assigned, "assigned_node_mismatch"
}

// shouldProcessOnNode returns true when the object is assigned to this node
// or has no explicit node assignment (for backward compatibility).
func shouldProcessOnNode(metadata *api.Metadata, nodeName string) bool {
	ok, _, _ := evaluateNodeAssignment(metadata, nodeName)
	return ok
}
