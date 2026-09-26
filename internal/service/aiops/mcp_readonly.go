package aiops

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/util/validation"
)

// NewReadOnlyService exposes existing query tools without requiring an LLM configuration.
func NewReadOnlyService(db *gorm.DB) *Service { return &Service{db: db} }

// ExecuteMCPReadOnly is the sole MCP bridge to Agent tools. Keep the allowlist here:
// MCP callers must never reach mutation, shell, logs, secrets, or full Pod manifests.
func (s *Service) ExecuteMCPReadOnly(ctx context.Context, userID, clusterID uint, toolName, argsJSON string) (string, error) {
	var args struct {
		ResourceType string `json:"resource_type"`
		Namespace    string `json:"namespace"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid tool arguments: %w", err)
	}
	if clusterID == 0 || len(validation.IsDNS1123Label(args.Namespace)) != 0 {
		return "", fmt.Errorf("cluster_id and a concrete namespace are required")
	}
	switch toolName {
	case "list_workloads":
		if !mcpWorkloadType(args.ResourceType) {
			return "", fmt.Errorf("resource_type must be pods, deployments, or services")
		}
		toolName = "list_resources"
	case "list_events":
		toolName = "get_events"
	default:
		return "", fmt.Errorf("MCP tool is not allowed")
	}
	if s == nil || s.db == nil {
		return "", fmt.Errorf("read-only tool service is unavailable")
	}
	result := s.executeAgentTool(ctx, userID, clusterID, 0, toolName, argsJSON)
	if result.IsError {
		return "", fmt.Errorf("%s", result.Content)
	}
	return result.Content, nil
}

func mcpWorkloadType(resourceType string) bool {
	switch strings.ToLower(strings.TrimSpace(resourceType)) {
	case "pods", "deployments", "services":
		return true
	default:
		return false
	}
}
