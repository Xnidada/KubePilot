package aiops

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/llm"
	"github.com/kubepilot/kubepilot/internal/model"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	toolResultMaxChars = 8000
	agentMaxToolRounds = 8
)

// ToolTraceItem is one tool invocation exposed to the UI.
type ToolTraceItem struct {
	Name    string `json:"name"`
	Args    string `json:"args"`
	Result  string `json:"result"`
	IsError bool   `json:"is_error"`
}

// PendingActionInfo is a staged write action awaiting user confirmation.
type PendingActionInfo struct {
	ID          uint   `json:"id"`
	ActionID    uint   `json:"action_id"` // alias for UI confirm API
	Action      string `json:"action"`
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Description string `json:"description"`
	DryRun      string `json:"dry_run"`
	NeedConfirm bool   `json:"need_confirm"`
}

func agentToolDefinitions() []llm.ToolDefinition {
	return []llm.ToolDefinition{
		llm.NewFunctionTool("list_resources",
			"List Kubernetes resources in a cluster. Prefer a namespace filter. resource_type: pods|deployments|services|namespaces|nodes|configmaps|secrets|events",
			`{"type":"object","properties":{"resource_type":{"type":"string"},"namespace":{"type":"string"},"label_selector":{"type":"string"},"limit":{"type":"integer"}},"required":["resource_type"]}`),
		llm.NewFunctionTool("get_resource",
			"Get one Kubernetes resource by name. resource_type: pod|deployment|service|configmap|secret|namespace|node",
			`{"type":"object","properties":{"resource_type":{"type":"string"},"namespace":{"type":"string"},"name":{"type":"string"}},"required":["resource_type","name"]}`),
		llm.NewFunctionTool("get_events",
			"List Warning/Normal events, optionally filtered by namespace and involved object name.",
			`{"type":"object","properties":{"namespace":{"type":"string"},"name":{"type":"string"},"limit":{"type":"integer"}}}`),
		llm.NewFunctionTool("get_pod_logs",
			"Fetch recent logs from a Pod. Do not invent pod names — use list_resources/get_resource first.",
			`{"type":"object","properties":{"namespace":{"type":"string"},"name":{"type":"string"},"container":{"type":"string"},"tail_lines":{"type":"integer"}},"required":["namespace","name"]}`),
		llm.NewFunctionTool("describe_resource",
			"Describe a resource: status, conditions, and related recent events. resource_type: pod|deployment|service|node",
			`{"type":"object","properties":{"resource_type":{"type":"string"},"namespace":{"type":"string"},"name":{"type":"string"}},"required":["resource_type","name"]}`),
		llm.NewFunctionTool("propose_mutation",
			"Dry-run preview of a mutating action WITHOUT staging. action: create_deployment|create_service|delete_deployment|delete_service|delete_pod|scale_deployment. Use after you have accurate names from tools.",
			`{"type":"object","properties":{"action":{"type":"string"},"namespace":{"type":"string"},"name":{"type":"string"},"image":{"type":"string"},"replicas":{"type":"integer"},"ports":{"type":"array","items":{"type":"integer"}},"service_type":{"type":"string"},"port":{"type":"integer"},"target_port":{"type":"integer"},"node_port":{"type":"integer"},"selector":{"type":"object","additionalProperties":{"type":"string"}}},"required":["action","namespace","name"]}`),
		llm.NewFunctionTool("stage_mutation",
			"Stage a mutating action for user confirmation (does NOT apply yet). Same action types as propose_mutation. After staging, wait for the user to confirm in the UI.",
			`{"type":"object","properties":{"action":{"type":"string"},"namespace":{"type":"string"},"name":{"type":"string"},"image":{"type":"string"},"replicas":{"type":"integer"},"ports":{"type":"array","items":{"type":"integer"}},"service_type":{"type":"string"},"port":{"type":"integer"},"target_port":{"type":"integer"},"node_port":{"type":"integer"},"selector":{"type":"object","additionalProperties":{"type":"string"}},"description":{"type":"string"}},"required":["action","namespace","name"]}`),
	}
}

type toolExecResult struct {
	Content    string
	IsError    bool
	Pending    *PendingActionInfo
	StopLoop   bool // after staging writes, prefer ending tool rounds
}

func (s *Service) executeAgentTool(ctx context.Context, userID, clusterID, conversationID uint, name, argsJSON string) toolExecResult {
	switch name {
	case "list_resources":
		return s.toolListResources(ctx, clusterID, argsJSON)
	case "get_resource":
		return s.toolGetResource(ctx, clusterID, argsJSON)
	case "get_events":
		return s.toolGetEvents(ctx, clusterID, argsJSON)
	case "get_pod_logs":
		return s.toolGetPodLogs(ctx, clusterID, argsJSON)
	case "describe_resource":
		return s.toolDescribeResource(ctx, clusterID, argsJSON)
	case "propose_mutation":
		return s.toolProposeMutation(ctx, clusterID, argsJSON)
	case "stage_mutation":
		return s.toolStageMutation(ctx, userID, clusterID, conversationID, argsJSON)
	default:
		return toolExecResult{Content: fmt.Sprintf("unknown tool: %s", name), IsError: true}
	}
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "\n...(truncated=true)"
}

func parseToolArgs(argsJSON string, dst interface{}) error {
	if strings.TrimSpace(argsJSON) == "" {
		argsJSON = "{}"
	}
	return json.Unmarshal([]byte(argsJSON), dst)
}

func (s *Service) toolListResources(ctx context.Context, clusterID uint, argsJSON string) toolExecResult {
	var args struct {
		ResourceType  string `json:"resource_type"`
		Namespace     string `json:"namespace"`
		LabelSelector string `json:"label_selector"`
		Limit         int    `json:"limit"`
	}
	if err := parseToolArgs(argsJSON, &args); err != nil {
		return toolExecResult{Content: err.Error(), IsError: true}
	}
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return toolExecResult{Content: err.Error(), IsError: true}
	}
	limit := args.Limit
	if limit <= 0 || limit > 50 {
		limit = 30
	}
	opts := metav1.ListOptions{LabelSelector: args.LabelSelector}
	rt := strings.ToLower(strings.TrimSpace(args.ResourceType))
	var b strings.Builder
	truncated := false
	total := 0

	switch rt {
	case "pod", "pods":
		list, err := client.Clientset.CoreV1().Pods(args.Namespace).List(ctx, opts)
		if err != nil {
			return toolExecResult{Content: err.Error(), IsError: true}
		}
		total = len(list.Items)
		b.WriteString(fmt.Sprintf("pods total=%d showing=%d ns=%q\n", total, min(total, limit), args.Namespace))
		for i, p := range list.Items {
			if i >= limit {
				truncated = true
				break
			}
			restarts := int32(0)
			for _, cs := range p.Status.ContainerStatuses {
				restarts += cs.RestartCount
			}
			b.WriteString(fmt.Sprintf("- %s/%s phase=%s restarts=%d node=%s\n", p.Namespace, p.Name, p.Status.Phase, restarts, p.Spec.NodeName))
		}
	case "deployment", "deployments", "deploy":
		list, err := client.Clientset.AppsV1().Deployments(args.Namespace).List(ctx, opts)
		if err != nil {
			return toolExecResult{Content: err.Error(), IsError: true}
		}
		total = len(list.Items)
		b.WriteString(fmt.Sprintf("deployments total=%d showing=%d ns=%q\n", total, min(total, limit), args.Namespace))
		for i, d := range list.Items {
			if i >= limit {
				truncated = true
				break
			}
			replicas := int32(0)
			if d.Spec.Replicas != nil {
				replicas = *d.Spec.Replicas
			}
			b.WriteString(fmt.Sprintf("- %s/%s ready=%d/%d\n", d.Namespace, d.Name, d.Status.ReadyReplicas, replicas))
		}
	case "service", "services", "svc":
		list, err := client.Clientset.CoreV1().Services(args.Namespace).List(ctx, opts)
		if err != nil {
			return toolExecResult{Content: err.Error(), IsError: true}
		}
		total = len(list.Items)
		b.WriteString(fmt.Sprintf("services total=%d showing=%d ns=%q\n", total, min(total, limit), args.Namespace))
		for i, svc := range list.Items {
			if i >= limit {
				truncated = true
				break
			}
			b.WriteString(fmt.Sprintf("- %s/%s type=%s clusterIP=%s\n", svc.Namespace, svc.Name, svc.Spec.Type, svc.Spec.ClusterIP))
		}
	case "namespace", "namespaces", "ns":
		list, err := client.Clientset.CoreV1().Namespaces().List(ctx, opts)
		if err != nil {
			return toolExecResult{Content: err.Error(), IsError: true}
		}
		total = len(list.Items)
		b.WriteString(fmt.Sprintf("namespaces total=%d showing=%d\n", total, min(total, limit)))
		for i, ns := range list.Items {
			if i >= limit {
				truncated = true
				break
			}
			b.WriteString(fmt.Sprintf("- %s phase=%s\n", ns.Name, ns.Status.Phase))
		}
	case "node", "nodes":
		list, err := client.Clientset.CoreV1().Nodes().List(ctx, opts)
		if err != nil {
			return toolExecResult{Content: err.Error(), IsError: true}
		}
		total = len(list.Items)
		b.WriteString(fmt.Sprintf("nodes total=%d showing=%d\n", total, min(total, limit)))
		for i, n := range list.Items {
			if i >= limit {
				truncated = true
				break
			}
			ready := "Unknown"
			for _, c := range n.Status.Conditions {
				if c.Type == corev1.NodeReady {
					ready = string(c.Status)
					break
				}
			}
			b.WriteString(fmt.Sprintf("- %s ready=%s\n", n.Name, ready))
		}
	case "configmap", "configmaps", "cm":
		list, err := client.Clientset.CoreV1().ConfigMaps(args.Namespace).List(ctx, opts)
		if err != nil {
			return toolExecResult{Content: err.Error(), IsError: true}
		}
		total = len(list.Items)
		b.WriteString(fmt.Sprintf("configmaps total=%d showing=%d ns=%q\n", total, min(total, limit), args.Namespace))
		for i, cm := range list.Items {
			if i >= limit {
				truncated = true
				break
			}
			b.WriteString(fmt.Sprintf("- %s/%s keys=%d\n", cm.Namespace, cm.Name, len(cm.Data)))
		}
	case "secret", "secrets":
		list, err := client.Clientset.CoreV1().Secrets(args.Namespace).List(ctx, opts)
		if err != nil {
			return toolExecResult{Content: err.Error(), IsError: true}
		}
		total = len(list.Items)
		b.WriteString(fmt.Sprintf("secrets total=%d showing=%d ns=%q (values redacted)\n", total, min(total, limit), args.Namespace))
		for i, sec := range list.Items {
			if i >= limit {
				truncated = true
				break
			}
			b.WriteString(fmt.Sprintf("- %s/%s type=%s keys=%d\n", sec.Namespace, sec.Name, sec.Type, len(sec.Data)))
		}
	case "event", "events":
		return s.toolGetEvents(ctx, clusterID, argsJSON)
	default:
		return toolExecResult{Content: fmt.Sprintf("unsupported resource_type %q", args.ResourceType), IsError: true}
	}
	if truncated {
		b.WriteString(fmt.Sprintf("truncated=true total=%d limit=%d\n", total, limit))
	}
	return toolExecResult{Content: truncateRunes(b.String(), toolResultMaxChars)}
}

func (s *Service) toolGetResource(ctx context.Context, clusterID uint, argsJSON string) toolExecResult {
	var args struct {
		ResourceType string `json:"resource_type"`
		Namespace    string `json:"namespace"`
		Name         string `json:"name"`
	}
	if err := parseToolArgs(argsJSON, &args); err != nil {
		return toolExecResult{Content: err.Error(), IsError: true}
	}
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return toolExecResult{Content: err.Error(), IsError: true}
	}
	rt := strings.ToLower(strings.TrimSpace(args.ResourceType))
	ns := args.Namespace
	name := args.Name
	var raw interface{}
	switch rt {
	case "pod", "pods":
		raw, err = client.Clientset.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
	case "deployment", "deployments", "deploy":
		raw, err = client.Clientset.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
	case "service", "services", "svc":
		raw, err = client.Clientset.CoreV1().Services(ns).Get(ctx, name, metav1.GetOptions{})
	case "configmap", "configmaps", "cm":
		raw, err = client.Clientset.CoreV1().ConfigMaps(ns).Get(ctx, name, metav1.GetOptions{})
	case "secret", "secrets":
		sec, e := client.Clientset.CoreV1().Secrets(ns).Get(ctx, name, metav1.GetOptions{})
		if e != nil {
			return toolExecResult{Content: e.Error(), IsError: true}
		}
		keys := make([]string, 0, len(sec.Data))
		for k, v := range sec.Data {
			keys = append(keys, fmt.Sprintf("%s(len=%d)", k, len(v)))
		}
		return toolExecResult{Content: truncateRunes(fmt.Sprintf("secret %s/%s type=%s keys=%v (values redacted)", ns, name, sec.Type, keys), toolResultMaxChars)}
	case "namespace", "namespaces", "ns":
		raw, err = client.Clientset.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	case "node", "nodes":
		raw, err = client.Clientset.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
	default:
		return toolExecResult{Content: fmt.Sprintf("unsupported resource_type %q", args.ResourceType), IsError: true}
	}
	if err != nil {
		return toolExecResult{Content: err.Error(), IsError: true}
	}
	b, _ := json.MarshalIndent(raw, "", "  ")
	return toolExecResult{Content: truncateRunes(string(b), toolResultMaxChars)}
}

func (s *Service) toolGetEvents(ctx context.Context, clusterID uint, argsJSON string) toolExecResult {
	var args struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
		Limit     int    `json:"limit"`
	}
	if err := parseToolArgs(argsJSON, &args); err != nil {
		return toolExecResult{Content: err.Error(), IsError: true}
	}
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return toolExecResult{Content: err.Error(), IsError: true}
	}
	limit := args.Limit
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	list, err := client.Clientset.CoreV1().Events(args.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return toolExecResult{Content: err.Error(), IsError: true}
	}
	var b strings.Builder
	count := 0
	for i := len(list.Items) - 1; i >= 0; i-- {
		ev := list.Items[i]
		if args.Name != "" && !strings.EqualFold(ev.InvolvedObject.Name, args.Name) {
			continue
		}
		b.WriteString(fmt.Sprintf("- [%s] %s %s/%s reason=%s msg=%s\n",
			ev.Type, ev.InvolvedObject.Kind, ev.Namespace, ev.InvolvedObject.Name, ev.Reason, ev.Message))
		count++
		if count >= limit {
			break
		}
	}
	if count == 0 {
		b.WriteString("no events matched\n")
	}
	return toolExecResult{Content: truncateRunes(b.String(), toolResultMaxChars)}
}

func (s *Service) toolGetPodLogs(ctx context.Context, clusterID uint, argsJSON string) toolExecResult {
	var args struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
		Container string `json:"container"`
		TailLines int64  `json:"tail_lines"`
	}
	if err := parseToolArgs(argsJSON, &args); err != nil {
		return toolExecResult{Content: err.Error(), IsError: true}
	}
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return toolExecResult{Content: err.Error(), IsError: true}
	}
	tail := args.TailLines
	if tail <= 0 || tail > 200 {
		tail = 80
	}
	opts := &corev1.PodLogOptions{TailLines: &tail}
	if args.Container != "" {
		opts.Container = args.Container
	}
	req := client.Clientset.CoreV1().Pods(args.Namespace).GetLogs(args.Name, opts)
	data, err := req.Do(ctx).Raw()
	if err != nil {
		return toolExecResult{Content: err.Error(), IsError: true}
	}
	return toolExecResult{Content: truncateRunes(string(data), toolResultMaxChars)}
}

func (s *Service) toolDescribeResource(ctx context.Context, clusterID uint, argsJSON string) toolExecResult {
	// Reuse get + events for a compact describe-like view.
	getRes := s.toolGetResource(ctx, clusterID, argsJSON)
	if getRes.IsError {
		return getRes
	}
	var args struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
	}
	_ = parseToolArgs(argsJSON, &args)
	evArgs, _ := json.Marshal(map[string]interface{}{"namespace": args.Namespace, "name": args.Name, "limit": 15})
	evRes := s.toolGetEvents(ctx, clusterID, string(evArgs))
	out := "=== resource ===\n" + getRes.Content + "\n=== events ===\n" + evRes.Content
	return toolExecResult{Content: truncateRunes(out, toolResultMaxChars)}
}

func parseMutationParams(argsJSON string) (StagedActionParams, error) {
	var params StagedActionParams
	if err := parseToolArgs(argsJSON, &params); err != nil {
		return params, err
	}
	params.Action = strings.TrimSpace(params.Action)
	if params.Action == "" {
		return params, fmt.Errorf("action is required")
	}
	if params.Namespace == "" {
		params.Namespace = "default"
	}
	if params.Name == "" {
		return params, fmt.Errorf("name is required")
	}
	return params, nil
}

func validateMutationParams(params StagedActionParams) error {
	switch params.Action {
	case "create_deployment":
		if strings.TrimSpace(params.Image) == "" {
			return fmt.Errorf("create_deployment requires image (do not invent defaults)")
		}
	case "scale_deployment":
		if params.Replicas < 0 {
			return fmt.Errorf("scale_deployment requires replicas >= 0")
		}
	}
	return nil
}

func (s *Service) toolProposeMutation(ctx context.Context, clusterID uint, argsJSON string) toolExecResult {
	params, err := parseMutationParams(argsJSON)
	if err != nil {
		return toolExecResult{Content: err.Error(), IsError: true}
	}
	if err := validateMutationParams(params); err != nil {
		return toolExecResult{Content: err.Error(), IsError: true}
	}
	dry, err := s.DryRunStagedAction(ctx, clusterID, params)
	if err != nil {
		return toolExecResult{Content: err.Error(), IsError: true}
	}
	return toolExecResult{Content: dry + "\n(note: propose_mutation does not stage; call stage_mutation to request user confirmation)"}
}

func (s *Service) toolStageMutation(ctx context.Context, userID, clusterID, conversationID uint, argsJSON string) toolExecResult {
	params, err := parseMutationParams(argsJSON)
	if err != nil {
		return toolExecResult{Content: err.Error(), IsError: true}
	}
	if err := validateMutationParams(params); err != nil {
		return toolExecResult{Content: err.Error(), IsError: true}
	}
	var meta struct {
		Description string `json:"description"`
	}
	_ = parseToolArgs(argsJSON, &meta)

	dry, err := s.DryRunStagedAction(ctx, clusterID, params)
	if err != nil {
		return toolExecResult{Content: err.Error(), IsError: true}
	}

	paramBytes, _ := json.Marshal(params)
	desc := strings.TrimSpace(meta.Description)
	if desc == "" {
		desc = fmt.Sprintf("%s %s/%s", params.Action, params.Namespace, params.Name)
	}
	actionType := "update"
	switch {
	case strings.HasPrefix(params.Action, "create"):
		actionType = "create"
	case strings.HasPrefix(params.Action, "delete"):
		actionType = "delete"
	case strings.HasPrefix(params.Action, "scale"):
		actionType = "scale"
	}
	rec := model.AgentAction{
		UserID:       userID,
		ActionType:   actionType,
		ResourceType: params.Action,
		ResourceName: params.Name,
		Namespace:    params.Namespace,
		ClusterID:    clusterID,
		Description:  desc,
		Parameters:   string(paramBytes),
		DryRunResult: dry,
		Status:       "pending",
		CreatedAt:    time.Now(),
	}
	if conversationID > 0 {
		cid := conversationID
		rec.ConversationID = &cid
	}
	if err := s.db.Create(&rec).Error; err != nil {
		return toolExecResult{Content: "failed to stage action: " + err.Error(), IsError: true}
	}
	pending := &PendingActionInfo{
		ID:          rec.ID,
		ActionID:    rec.ID,
		Action:      params.Action,
		Name:        params.Name,
		Namespace:   params.Namespace,
		Description: desc,
		DryRun:      dry,
		NeedConfirm: true,
	}
	return toolExecResult{
		Content: fmt.Sprintf("staged action_id=%d status=pending\n%s\nAwaiting user confirmation in UI. If more resources need staging, call stage_mutation again for each.", rec.ID, dry),
		Pending: pending,
		// Do not stop the loop: batch writes often need multiple stage_mutation calls across rounds.
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
