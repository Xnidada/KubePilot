package aiops

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func isRetryableQueryTool(name string) bool {
	switch name {
	case "list_resources", "get_resource", "get_events", "get_pod_logs", "describe_resource", "diagnose_workload", "diagnose_service":
		return true
	}
	return false
}

// RetryAgentQuery only accepts pure query tools, never a dry-run or staged write.
func (s *Service) RetryAgentQuery(ctx context.Context, userID, clusterID, conversationID uint, name, args string) (ToolTraceItem, error) {
	if !isRetryableQueryTool(name) {
		return ToolTraceItem{}, fmt.Errorf("only read-only query tools can be retried from the UI")
	}
	if len(args) > 8192 {
		return ToolTraceItem{}, fmt.Errorf("query arguments too long")
	}
	started := time.Now()
	result := retryAgentTool(ctx, name, func() toolExecResult {
		return s.executeAgentTool(ctx, userID, clusterID, conversationID, name, args)
	})
	return ToolTraceItem{Name: name, Args: args, Result: truncateRunes(result.Content, toolResultMaxChars),
		IsError: result.IsError, DurationMs: time.Since(started).Milliseconds()}, nil
}

// Transient failures are the only failures worth repeating. Validation, RBAC and
// NotFound errors need to go back to the model/user rather than burn retries.
func isTransientAgentFailure(msg string) bool {
	lower := strings.ToLower(msg)
	for _, marker := range []string{
		"timeout", "deadline exceeded", "connection reset", "connection refused",
		"broken pipe", "unexpected eof", "temporarily unavailable", "service unavailable",
		"too many requests", "status 429", "status 500", "status 502", "status 503", "status 504",
		"server is currently unable", "etcdserver: request timed out",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func retryAgentTool(ctx context.Context, name string, run func() toolExecResult) toolExecResult {
	var result toolExecResult
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return toolExecResult{Content: err.Error(), IsError: true}
		}
		result = run()
		if !result.IsError || !isTransientAgentFailure(result.Content) || attempt == 2 {
			return result
		}
		// Stage tools only create pending records, never apply a cluster mutation.
		// stageOneMutation checks for an identical pending action before inserting.
		if !isReadOnlyTool(name) && name != "stage_mutation" && name != "stage_mutations" && name != "delete_by_prefix" {
			return result
		}
		delay := time.Duration(250<<attempt) * time.Millisecond
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return toolExecResult{Content: ctx.Err().Error(), IsError: true}
		case <-timer.C:
		}
	}
	return result
}
