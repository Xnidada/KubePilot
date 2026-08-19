package aiops

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/llm"
	"github.com/kubepilot/kubepilot/internal/model"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	toolResultMaxChars = 16000
	agentMaxToolRounds = 15
)

// ToolTraceItem is one tool invocation exposed to the UI.
type ToolTraceItem struct {
	Name       string `json:"name"`
	Args       string `json:"args"`
	Result     string `json:"result"`
	IsError    bool   `json:"is_error"`
	DurationMs int64  `json:"duration_ms,omitempty"`
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
			"List Kubernetes resources with filters. Prefer namespace. Use name_prefix or label_selector to avoid cluster-wide scans. resource_type: pods|deployments|services|namespaces|nodes|configmaps|secrets|events. Returns truncated/total metadata.",
			`{"type":"object","properties":{"resource_type":{"type":"string"},"namespace":{"type":"string"},"name_prefix":{"type":"string"},"label_selector":{"type":"string"},"field_selector":{"type":"string"},"limit":{"type":"integer"}},"required":["resource_type"]}`),
		llm.NewFunctionTool("get_resource",
			"Get one resource. For Service returns compact connectivity summary (type/ports/nodePort/selector/endpoints/matching pods). Do NOT invent nodePort — quote tool output.",
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
		llm.NewFunctionTool("diagnose_workload",
			"Run a fixed diagnosis pipeline for a Pod/Deployment (get + events + logs/status). Use for CrashLoop/Pending/ImagePull errors.",
			`{"type":"object","properties":{"resource_type":{"type":"string","description":"pod or deployment"},"namespace":{"type":"string"},"name":{"type":"string"},"tail_lines":{"type":"integer"}},"required":["resource_type","namespace","name"]}`),
		llm.NewFunctionTool("diagnose_service",
			"Diagnose ANY Service access issue (NodePort/ClusterIP). Pass name+namespace, OR node_port (optionally namespace) to auto-resolve. Returns ports, Endpoints, selector match, and ranked selector near-miss vs Pods/Deployments — no app-specific assumptions.",
			`{"type":"object","properties":{"namespace":{"type":"string"},"name":{"type":"string"},"node_port":{"type":"integer"}},"required":[]}`),
		llm.NewFunctionTool("propose_mutation",
			"Dry-run preview of a mutating action WITHOUT staging. action: create_deployment|create_service|delete_deployment|delete_service|delete_pod|scale_deployment. For host mounts on create_deployment use host_path_mounts.",
			`{"type":"object","properties":{"action":{"type":"string"},"namespace":{"type":"string"},"name":{"type":"string"},"image":{"type":"string"},"replicas":{"type":"integer"},"ports":{"type":"array","items":{"type":"integer"}},"host_path_mounts":{"type":"array","items":{"type":"object","properties":{"host_path":{"type":"string"},"mount_path":{"type":"string"},"read_only":{"type":"boolean"},"name":{"type":"string"},"type":{"type":"string"}},"required":["host_path","mount_path"]}},"service_type":{"type":"string"},"port":{"type":"integer"},"target_port":{"type":"integer"},"node_port":{"type":"integer"},"selector":{"type":"object","additionalProperties":{"type":"string"}}},"required":["action","namespace","name"]}`),
		llm.NewFunctionTool("stage_mutation",
			"Stage one mutating action for user confirmation (does NOT apply yet). Actions: create_deployment|create_service|delete_deployment|delete_service|delete_pod|scale_deployment|create_configmap|create_secret|update_deployment|create_namespace|create_ingress|create_hpa|create_pvc|apply_yaml. For external access: action=create_service, service_type=NodePort, and REQUIRED node_port (30000-32767). For hostPath mounts on create_deployment: REQUIRED host_path_mounts=[{host_path,mount_path}]. For apply_yaml: pass yaml field with raw Kubernetes YAML.",
			`{"type":"object","properties":{"action":{"type":"string"},"namespace":{"type":"string"},"name":{"type":"string"},"image":{"type":"string"},"replicas":{"type":"integer"},"ports":{"type":"array","items":{"type":"integer"}},"host_path_mounts":{"type":"array","items":{"type":"object","properties":{"host_path":{"type":"string"},"mount_path":{"type":"string"},"read_only":{"type":"boolean"},"name":{"type":"string"},"type":{"type":"string"}},"required":["host_path","mount_path"]}},"service_type":{"type":"string"},"port":{"type":"integer"},"target_port":{"type":"integer"},"node_port":{"type":"integer"},"selector":{"type":"object","additionalProperties":{"type":"string"}},"data":{"type":"object","additionalProperties":{"type":"string"},"description":"ConfigMap/Secret key-value data"},"secret_type":{"type":"string","description":"Secret type: Opaque(default), tls.io/IngressTLS, etc."},"new_image":{"type":"string","description":"update_deployment: new container image"},"env_vars":{"type":"object","additionalProperties":{"type":"string"},"description":"update_deployment: environment variable overrides"},"resource_limits":{"type":"object","additionalProperties":{"type":"string"},"description":"update_deployment: resource limits (cpu, memory)"},"host":{"type":"string","description":"Ingress host"},"path":{"type":"string","description":"Ingress path"},"backend_service":{"type":"string","description":"Ingress backend service name"},"backend_port":{"type":"integer","description":"Ingress backend service port"},"min_replicas":{"type":"integer","description":"HPA minReplicas"},"max_replicas":{"type":"integer","description":"HPA maxReplicas"},"target_cpu":{"type":"integer","description":"HPA target CPU utilization percentage"},"storage_class":{"type":"string","description":"PVC storageClassName"},"access_modes":{"type":"array","items":{"type":"string"},"description":"PVC accessModes e.g. ReadWriteOnce"},"storage_size":{"type":"string","description":"PVC size e.g. 10Gi"},"yaml":{"type":"string","description":"apply_yaml: raw Kubernetes YAML manifest(s)"},"description":{"type":"string"}},"required":["action","namespace","name"]}`),
		llm.NewFunctionTool("stage_mutations",
			"Stage multiple mutating actions in one call (batch). Each item same schema as stage_mutation.",
			`{"type":"object","properties":{"items":{"type":"array","items":{"type":"object","properties":{"action":{"type":"string"},"namespace":{"type":"string"},"name":{"type":"string"},"image":{"type":"string"},"replicas":{"type":"integer"},"ports":{"type":"array","items":{"type":"integer"}},"host_path_mounts":{"type":"array","items":{"type":"object","properties":{"host_path":{"type":"string"},"mount_path":{"type":"string"},"read_only":{"type":"boolean"},"name":{"type":"string"},"type":{"type":"string"}},"required":["host_path","mount_path"]}},"service_type":{"type":"string"},"port":{"type":"integer"},"target_port":{"type":"integer"},"node_port":{"type":"integer"},"selector":{"type":"object","additionalProperties":{"type":"string"}},"description":{"type":"string"}},"required":["action","namespace","name"]}}},"required":["items"]}`),
		llm.NewFunctionTool("delete_by_prefix",
			"Stage delete_pod for all Pods in a namespace whose name starts with name_prefix. Does NOT delete immediately — stages for UI confirmation.",
			`{"type":"object","properties":{"namespace":{"type":"string"},"name_prefix":{"type":"string"},"limit":{"type":"integer"}},"required":["namespace","name_prefix"]}`),
	}
}

type toolExecResult struct {
	Content     string
	IsError     bool
	Pending     *PendingActionInfo
	PendingList []PendingActionInfo // batch staging
	StopLoop    bool
}

func (s *Service) executeAgentTool(ctx context.Context, userID, clusterID, conversationID uint, name, argsJSON string) toolExecResult {
	ns := toolNamespaceFromArgs(argsJSON)
	// Cluster-wide list uses empty ns → require "*" grant.
	if name == "list_resources" && ns == "" {
		var a struct {
			ResourceType string `json:"resource_type"`
		}
		_ = parseToolArgs(argsJSON, &a)
		rt := strings.ToLower(a.ResourceType)
		if rt != "namespace" && rt != "namespaces" && rt != "ns" && rt != "node" && rt != "nodes" {
			if err := s.ensureToolAccess(ctx, userID, clusterID, "*", false); err != nil {
				return toolExecResult{Content: "refusing cluster-wide list without cluster-scope grant (set namespace or name_prefix): " + err.Error(), IsError: true}
			}
		}
	}
	if err := s.ensureToolAccess(ctx, userID, clusterID, ns, toolIsWrite(name)); err != nil {
		return toolExecResult{Content: err.Error(), IsError: true}
	}

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
	case "diagnose_workload":
		return s.toolDiagnoseWorkload(ctx, clusterID, argsJSON)
	case "diagnose_service":
		return s.toolDiagnoseService(ctx, clusterID, argsJSON)
	case "propose_mutation":
		return s.toolProposeMutation(ctx, clusterID, argsJSON)
	case "stage_mutation":
		return s.toolStageMutation(ctx, userID, clusterID, conversationID, argsJSON)
	case "stage_mutations":
		return s.toolStageMutations(ctx, userID, clusterID, conversationID, argsJSON)
	case "delete_by_prefix":
		return s.toolDeleteByPrefix(ctx, userID, clusterID, conversationID, argsJSON)
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
	// Prefer cutting at a newline so resource names are never mid-sliced (e.g. "ngi" / "kube").
	cut := max
	for i := max; i > max*3/4 && i > 0; i-- {
		if r[i-1] == '\n' {
			cut = i
			break
		}
	}
	return string(r[:cut]) + "\n...(truncated=true)"
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
		NamePrefix    string `json:"name_prefix"`
		LabelSelector string `json:"label_selector"`
		FieldSelector string `json:"field_selector"`
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
	prefix := strings.TrimSpace(args.NamePrefix)
	opts := metav1.ListOptions{
		LabelSelector: args.LabelSelector,
		FieldSelector: args.FieldSelector,
	}
	rt := strings.ToLower(strings.TrimSpace(args.ResourceType))
	namespaced := rt != "namespace" && rt != "namespaces" && rt != "ns" && rt != "node" && rt != "nodes" && rt != "event" && rt != "events"
	if namespaced && strings.TrimSpace(args.Namespace) == "" && prefix == "" && args.LabelSelector == "" && args.FieldSelector == "" {
		return toolExecResult{
			Content: "refusing unfiltered cluster-wide list; set namespace and/or name_prefix/label_selector/field_selector",
			IsError: true,
		}
	}

	var lines []string
	switch rt {
	case "pod", "pods":
		list, err := client.Clientset.CoreV1().Pods(args.Namespace).List(ctx, opts)
		if err != nil {
			return toolExecResult{Content: err.Error(), IsError: true}
		}
		for _, p := range list.Items {
			if prefix != "" && !strings.HasPrefix(p.Name, prefix) {
				continue
			}
			restarts := int32(0)
			for _, cs := range p.Status.ContainerStatuses {
				restarts += cs.RestartCount
			}
			lines = append(lines, fmt.Sprintf("- %s/%s phase=%s restarts=%d node=%s", p.Namespace, p.Name, p.Status.Phase, restarts, p.Spec.NodeName))
		}
	case "deployment", "deployments", "deploy":
		list, err := client.Clientset.AppsV1().Deployments(args.Namespace).List(ctx, opts)
		if err != nil {
			return toolExecResult{Content: err.Error(), IsError: true}
		}
		for _, d := range list.Items {
			if prefix != "" && !strings.HasPrefix(d.Name, prefix) {
				continue
			}
			replicas := int32(0)
			if d.Spec.Replicas != nil {
				replicas = *d.Spec.Replicas
			}
			lines = append(lines, fmt.Sprintf("- %s/%s ready=%d/%d", d.Namespace, d.Name, d.Status.ReadyReplicas, replicas))
		}
	case "service", "services", "svc":
		list, err := client.Clientset.CoreV1().Services(args.Namespace).List(ctx, opts)
		if err != nil {
			return toolExecResult{Content: err.Error(), IsError: true}
		}
		for _, svc := range list.Items {
			if prefix != "" && !strings.HasPrefix(svc.Name, prefix) {
				continue
			}
			epReady := -1
			if eps, eerr := client.Clientset.CoreV1().Endpoints(svc.Namespace).Get(ctx, svc.Name, metav1.GetOptions{}); eerr == nil {
				epReady = 0
				for _, subset := range eps.Subsets {
					epReady += len(subset.Addresses)
				}
			}
			lines = append(lines, fmt.Sprintf("- %s/%s type=%s clusterIP=%s ports=[%s] endpoints_ready=%d selector=%v",
				svc.Namespace, svc.Name, svc.Spec.Type, svc.Spec.ClusterIP, formatServicePorts(svc.Spec.Ports), epReady, svc.Spec.Selector))
		}
	case "namespace", "namespaces", "ns":
		list, err := client.Clientset.CoreV1().Namespaces().List(ctx, opts)
		if err != nil {
			return toolExecResult{Content: err.Error(), IsError: true}
		}
		for _, ns := range list.Items {
			if prefix != "" && !strings.HasPrefix(ns.Name, prefix) {
				continue
			}
			lines = append(lines, fmt.Sprintf("- %s phase=%s", ns.Name, ns.Status.Phase))
		}
	case "node", "nodes":
		list, err := client.Clientset.CoreV1().Nodes().List(ctx, opts)
		if err != nil {
			return toolExecResult{Content: err.Error(), IsError: true}
		}
		for _, n := range list.Items {
			if prefix != "" && !strings.HasPrefix(n.Name, prefix) {
				continue
			}
			ready := "Unknown"
			for _, c := range n.Status.Conditions {
				if c.Type == corev1.NodeReady {
					ready = string(c.Status)
					break
				}
			}
			lines = append(lines, fmt.Sprintf("- %s ready=%s", n.Name, ready))
		}
	case "configmap", "configmaps", "cm":
		list, err := client.Clientset.CoreV1().ConfigMaps(args.Namespace).List(ctx, opts)
		if err != nil {
			return toolExecResult{Content: err.Error(), IsError: true}
		}
		for _, cm := range list.Items {
			if prefix != "" && !strings.HasPrefix(cm.Name, prefix) {
				continue
			}
			lines = append(lines, fmt.Sprintf("- %s/%s keys=%d", cm.Namespace, cm.Name, len(cm.Data)))
		}
	case "secret", "secrets":
		list, err := client.Clientset.CoreV1().Secrets(args.Namespace).List(ctx, opts)
		if err != nil {
			return toolExecResult{Content: err.Error(), IsError: true}
		}
		for _, sec := range list.Items {
			if prefix != "" && !strings.HasPrefix(sec.Name, prefix) {
				continue
			}
			lines = append(lines, fmt.Sprintf("- %s/%s type=%s keys=%d", sec.Namespace, sec.Name, sec.Type, len(sec.Data)))
		}
	case "event", "events":
		return s.toolGetEvents(ctx, clusterID, argsJSON)
	default:
		return toolExecResult{Content: fmt.Sprintf("unsupported resource_type %q", args.ResourceType), IsError: true}
	}

	total := len(lines)
	truncated := total > limit
	showing := total
	if truncated {
		showing = limit
		lines = lines[:limit]
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s total=%d showing=%d truncated=%v ns=%q name_prefix=%q label=%q field=%q\n",
		rt, total, showing, truncated, args.Namespace, prefix, args.LabelSelector, args.FieldSelector))
	for _, line := range lines {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	if truncated {
		b.WriteString(fmt.Sprintf("meta: {\"truncated\":true,\"total\":%d,\"limit\":%d}\n", total, limit))
	} else {
		b.WriteString(fmt.Sprintf("meta: {\"truncated\":false,\"total\":%d,\"limit\":%d}\n", total, limit))
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
		svc, e := client.Clientset.CoreV1().Services(ns).Get(ctx, name, metav1.GetOptions{})
		if e != nil {
			return toolExecResult{Content: e.Error(), IsError: true}
		}
		return toolExecResult{Content: truncateRunes(s.summarizeService(ctx, client, svc), toolResultMaxChars)}
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
	// Drop managedFields noise so ports/status stay within truncation window.
	if meta, ok := raw.(metav1.Object); ok {
		meta.SetManagedFields(nil)
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
	// Accept common aliases models invent (nodePort / NodePort).
	if params.NodePort == 0 {
		var aliases struct {
			NodePortCamel  int32 `json:"nodePort"`
			NodePortPascal int32 `json:"NodePort"`
		}
		_ = parseToolArgs(argsJSON, &aliases)
		if aliases.NodePortCamel > 0 {
			params.NodePort = aliases.NodePortCamel
		} else if aliases.NodePortPascal > 0 {
			params.NodePort = aliases.NodePortPascal
		}
	}
	if len(params.HostPathMounts) == 0 {
		var mountAliases struct {
			VolumeMounts        []HostPathMount `json:"volume_mounts"`
			HostPathMountsCamel []HostPathMount `json:"hostPathMounts"`
		}
		_ = parseToolArgs(argsJSON, &mountAliases)
		if len(mountAliases.VolumeMounts) > 0 {
			params.HostPathMounts = mountAliases.VolumeMounts
		} else if len(mountAliases.HostPathMountsCamel) > 0 {
			params.HostPathMounts = mountAliases.HostPathMountsCamel
		}
	}
	normalizeHostPathMounts(&params)
	params.Action = strings.TrimSpace(params.Action)
	if params.Action == "" {
		return params, fmt.Errorf("action is required")
	}
	if params.Name == "" {
		return params, fmt.Errorf("name is required")
	}
	// Creates may default ns; deletes/scales must be explicit (禁无 ns 删除).
	switch params.Action {
	case "delete_deployment", "delete_service", "delete_pod", "scale_deployment":
		// keep empty — validated below
	case "create_namespace":
		// namespace creation does not need a namespace field
	default:
		if params.Namespace == "" {
			params.Namespace = "default"
		}
	}
	if params.Action == "create_deployment" {
		params.Image = normalizeDeploymentImage(params.Image)
		if len(params.Ports) == 0 && strings.HasPrefix(strings.ToLower(params.Image), "nginx:") {
			params.Ports = []int32{80}
		}
	}
	normalizeServiceParams(&params)
	return params, nil
}

const pinnedNginxImage = "nginx:1.25.4"

func normalizeDeploymentImage(image string) string {
	img := strings.TrimSpace(image)
	lower := strings.ToLower(img)
	switch lower {
	case "nginx", "nginx:latest", "nginx:stable", "nginx:mainline":
		return pinnedNginxImage
	}
	if strings.HasPrefix(lower, "nginx:") {
		tag := strings.TrimPrefix(lower, "nginx:")
		// Models often pick floating tags (nginx:1.25) after latest is refused — pin patch.
		if tag == "1.25" || tag == "1.25-alpine" || tag == "1.27" || tag == "1.27-alpine" {
			if strings.Contains(tag, "alpine") {
				return "nginx:1.25.4-alpine"
			}
			return pinnedNginxImage
		}
	}
	return img
}

// extractHostPathMountHints infers hostPath mounts from natural-language paths.
func extractHostPathMountHints(msg string) []HostPathMount {
	paths := extractAbsolutePaths(msg)
	if len(paths) == 0 {
		return nil
	}
	out := make([]HostPathMount, 0, len(paths))
	seenHost := map[string]bool{}
	for _, hp := range paths {
		if seenHost[hp] {
			continue
		}
		seenHost[hp] = true
		mp := inferContainerMountPath(hp, msg)
		if mp == "" {
			continue
		}
		out = append(out, HostPathMount{HostPath: hp, MountPath: mp, Type: "DirectoryOrCreate"})
		if len(out) >= 8 {
			break
		}
	}
	return out
}

func extractAbsolutePaths(msg string) []string {
	var out []string
	for i := 0; i < len(msg); i++ {
		if msg[i] != '/' {
			continue
		}
		j := i + 1
		for j < len(msg) {
			c := msg[j]
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
				c == '_' || c == '-' || c == '.' || c == '/' {
				j++
				continue
			}
			break
		}
		p := strings.TrimRight(msg[i:j], "/.")
		if len(p) >= 2 && strings.Count(p, "/") >= 1 {
			out = append(out, p)
		}
		i = j
	}
	return out
}

func inferContainerMountPath(hostPath, msg string) string {
	lower := strings.ToLower(hostPath)
	switch {
	case strings.Contains(lower, "html") || strings.Contains(lower, "www") || strings.HasSuffix(lower, "/web"):
		return "/usr/share/nginx/html"
	case strings.Contains(lower, "log"):
		return "/var/log/nginx"
	}
	// Context near the path: 网页/主目录 → html; 日志 → log dir
	idx := strings.Index(msg, hostPath)
	if idx >= 0 {
		start := idx - 24
		if start < 0 {
			start = 0
		}
		ctx := msg[start:idx]
		if strings.Contains(ctx, "网页") || strings.Contains(ctx, "主目录") || strings.Contains(ctx, "html") {
			return "/usr/share/nginx/html"
		}
		if strings.Contains(ctx, "日志") || strings.Contains(strings.ToLower(ctx), "log") {
			return "/var/log/nginx"
		}
	}
	return ""
}

// enrichMutationArgsWithUserHints injects host_path_mounts (and pins image) when the model omits them.
func enrichMutationArgsWithUserHints(userMsg, argsJSON string) string {
	if strings.TrimSpace(argsJSON) == "" {
		return argsJSON
	}
	hints := extractHostPathMountHints(userMsg)
	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(argsJSON), &root); err != nil {
		return argsJSON
	}
	changed := false
	if rawItems, ok := root["items"]; ok {
		var items []json.RawMessage
		if err := json.Unmarshal(rawItems, &items); err == nil {
			for i, item := range items {
				enriched, did := enrichOneMutationJSON(string(item), hints, userMsg)
				if did {
					items[i] = json.RawMessage(enriched)
					changed = true
				}
			}
			if changed {
				b, _ := json.Marshal(items)
				root["items"] = b
			}
		}
	} else {
		enriched, did := enrichOneMutationJSON(argsJSON, hints, userMsg)
		if did {
			return enriched
		}
		return argsJSON
	}
	if !changed {
		return argsJSON
	}
	b, err := json.Marshal(root)
	if err != nil {
		return argsJSON
	}
	return string(b)
}

func enrichOneMutationJSON(argsJSON string, hints []HostPathMount, userMsg string) (string, bool) {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &m); err != nil {
		return argsJSON, false
	}
	action, _ := m["action"].(string)
	if action != "create_deployment" {
		return argsJSON, false
	}
	changed := false
	if img, ok := m["image"].(string); ok {
		norm := normalizeDeploymentImage(img)
		if norm != img {
			m["image"] = norm
			changed = true
		}
	} else if isHostMountIntent(userMsg) || strings.Contains(strings.ToLower(userMsg), "nginx") {
		m["image"] = pinnedNginxImage
		changed = true
	}
	hasMounts := false
	if _, ok := m["host_path_mounts"]; ok {
		hasMounts = true
	}
	if _, ok := m["volume_mounts"]; ok {
		hasMounts = true
	}
	if !hasMounts && len(hints) > 0 && isHostMountIntent(userMsg) {
		arr := make([]map[string]interface{}, 0, len(hints))
		for _, h := range hints {
			arr = append(arr, map[string]interface{}{
				"host_path":  h.HostPath,
				"mount_path": h.MountPath,
				"type":       "DirectoryOrCreate",
			})
		}
		m["host_path_mounts"] = arr
		changed = true
	}
	if ports, ok := m["ports"].([]interface{}); (!ok || len(ports) == 0) && strings.Contains(strings.ToLower(fmt.Sprint(m["image"])), "nginx") {
		m["ports"] = []int{80}
		changed = true
	}
	if !changed {
		return argsJSON, false
	}
	b, err := json.Marshal(m)
	if err != nil {
		return argsJSON, false
	}
	return string(b), true
}

func normalizeHostPathMounts(params *StagedActionParams) {
	if len(params.HostPathMounts) == 0 {
		return
	}
	out := make([]HostPathMount, 0, len(params.HostPathMounts))
	for _, m := range params.HostPathMounts {
		m.HostPath = strings.TrimSpace(m.HostPath)
		m.MountPath = strings.TrimSpace(m.MountPath)
		m.Name = strings.TrimSpace(m.Name)
		m.Type = strings.TrimSpace(m.Type)
		if m.HostPath == "" && m.MountPath == "" {
			continue
		}
		out = append(out, m)
	}
	params.HostPathMounts = out
}

func validateHostPathMounts(mounts []HostPathMount) error {
	if len(mounts) > 8 {
		return fmt.Errorf("host_path_mounts: max 8 mounts")
	}
	seen := map[string]bool{}
	for i, m := range mounts {
		if m.HostPath == "" || m.MountPath == "" {
			return fmt.Errorf("host_path_mounts[%d]: host_path and mount_path are required", i)
		}
		if !strings.HasPrefix(m.HostPath, "/") || !strings.HasPrefix(m.MountPath, "/") {
			return fmt.Errorf("host_path_mounts[%d]: paths must be absolute", i)
		}
		if strings.Contains(m.HostPath, "..") || strings.Contains(m.MountPath, "..") {
			return fmt.Errorf("host_path_mounts[%d]: path must not contain '..'", i)
		}
		if seen[m.MountPath] {
			return fmt.Errorf("host_path_mounts[%d]: duplicate mount_path %s", i, m.MountPath)
		}
		seen[m.MountPath] = true
		switch m.Type {
		case "", "Directory", "DirectoryOrCreate", "File", "FileOrCreate":
		default:
			return fmt.Errorf("host_path_mounts[%d]: unsupported type %q", i, m.Type)
		}
	}
	return nil
}

func normalizeServiceParams(params *StagedActionParams) {
	if params.Action != "create_service" {
		return
	}
	if params.NodePort > 0 {
		params.ServiceType = string(corev1.ServiceTypeNodePort)
	}
	st := strings.ToLower(strings.TrimSpace(params.ServiceType))
	switch st {
	case "nodeport":
		params.ServiceType = string(corev1.ServiceTypeNodePort)
	case "loadbalancer":
		params.ServiceType = string(corev1.ServiceTypeLoadBalancer)
	case "clusterip", "":
		if params.NodePort > 0 {
			params.ServiceType = string(corev1.ServiceTypeNodePort)
		} else if st == "clusterip" {
			params.ServiceType = string(corev1.ServiceTypeClusterIP)
		}
	}
	if params.TargetPort <= 0 && params.Port > 0 {
		params.TargetPort = params.Port
	}
}

func validateMutationParams(params StagedActionParams) error {
	switch params.Action {
	case "create_deployment":
		img := strings.TrimSpace(params.Image)
		if img == "" {
			return fmt.Errorf("create_deployment requires image (do not invent defaults)")
		}
		lower := strings.ToLower(img)
		// nginx / nginx:latest are normalized in parseMutationParams; refuse remaining placeholders.
		if lower == "nginx" || lower == "nginx:latest" || strings.Contains(lower, "example.com") {
			return fmt.Errorf("create_deployment: refuse placeholder image %q; ask the user for a real image", img)
		}
		if params.Replicas < 0 {
			return fmt.Errorf("create_deployment replicas must be >= 0")
		}
		if err := validateHostPathMounts(params.HostPathMounts); err != nil {
			return err
		}
	case "create_service":
		if len(params.Selector) == 0 {
			return fmt.Errorf("create_service requires selector (do not invent defaults)")
		}
		if params.Port <= 0 {
			return fmt.Errorf("create_service requires port > 0")
		}
		st := strings.ToLower(params.ServiceType)
		if st == "nodeport" || params.NodePort > 0 {
			if params.NodePort < 30000 || params.NodePort > 32767 {
				return fmt.Errorf("create_service NodePort requires node_port in 30000-32767 (got %d); do not omit — auto-assign is not accepted for staged NodePort", params.NodePort)
			}
		}
	case "scale_deployment":
		if strings.TrimSpace(params.Namespace) == "" {
			return fmt.Errorf("scale_deployment requires namespace")
		}
		if params.Replicas < 0 {
			return fmt.Errorf("scale_deployment requires replicas >= 0")
		}
	case "delete_deployment", "delete_service", "delete_pod":
		if strings.TrimSpace(params.Namespace) == "" {
			return fmt.Errorf("%s requires namespace (refuse delete without ns)", params.Action)
		}
	case "create_configmap":
		if strings.TrimSpace(params.Name) == "" {
			return fmt.Errorf("create_configmap requires name")
		}
		if len(params.Data) == 0 {
			return fmt.Errorf("create_configmap requires data (key-value pairs)")
		}
	case "create_secret":
		if strings.TrimSpace(params.Name) == "" {
			return fmt.Errorf("create_secret requires name")
		}
		if len(params.Data) == 0 {
			return fmt.Errorf("create_secret requires data (key-value pairs)")
		}
	case "update_deployment":
		if strings.TrimSpace(params.Namespace) == "" {
			return fmt.Errorf("update_deployment requires namespace")
		}
		if strings.TrimSpace(params.NewImage) == "" && len(params.EnvVars) == 0 && len(params.ResourceLimits) == 0 {
			return fmt.Errorf("update_deployment requires at least one of: new_image, env_vars, resource_limits")
		}
	case "create_namespace":
		if strings.TrimSpace(params.Name) == "" {
			return fmt.Errorf("create_namespace requires name")
		}
	case "create_ingress":
		if strings.TrimSpace(params.Namespace) == "" {
			return fmt.Errorf("create_ingress requires namespace")
		}
		if strings.TrimSpace(params.Host) == "" {
			return fmt.Errorf("create_ingress requires host")
		}
		if strings.TrimSpace(params.BackendService) == "" {
			return fmt.Errorf("create_ingress requires backend_service")
		}
	case "create_hpa":
		if strings.TrimSpace(params.Namespace) == "" {
			return fmt.Errorf("create_hpa requires namespace")
		}
		if params.MaxReplicas <= 0 {
			return fmt.Errorf("create_hpa requires max_replicas > 0")
		}
		if params.TargetCPU <= 0 {
			return fmt.Errorf("create_hpa requires target_cpu > 0")
		}
	case "create_pvc":
		if strings.TrimSpace(params.Namespace) == "" {
			return fmt.Errorf("create_pvc requires namespace")
		}
		if strings.TrimSpace(params.StorageSize) == "" {
			return fmt.Errorf("create_pvc requires storage_size (e.g. 10Gi)")
		}
	case "apply_yaml":
		if strings.TrimSpace(params.YAML) == "" {
			return fmt.Errorf("apply_yaml requires yaml content")
		}
	default:
		return fmt.Errorf("unsupported action: %s", params.Action)
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
	var meta struct {
		Description string `json:"description"`
	}
	_ = parseToolArgs(argsJSON, &meta)
	pending, dry, err := s.stageOneMutation(ctx, userID, clusterID, conversationID, params, meta.Description)
	if err != nil {
		return toolExecResult{Content: err.Error(), IsError: true}
	}
	return toolExecResult{
		Content: fmt.Sprintf("staged action_id=%d status=pending\n%s\nAwaiting user confirmation in UI.", pending.ID, dry),
		Pending: pending,
	}
}

func (s *Service) stageOneMutation(ctx context.Context, userID, clusterID, conversationID uint, params StagedActionParams, description string) (*PendingActionInfo, string, error) {
	if err := validateMutationParams(params); err != nil {
		return nil, "", err
	}
	dry, err := s.DryRunStagedAction(ctx, clusterID, params)
	if err != nil {
		return nil, "", err
	}
	// Supersede earlier pending create for the same resource in this conversation.
	if conversationID > 0 && (params.Action == "create_deployment" || params.Action == "create_service") {
		_ = s.db.Model(&model.AgentAction{}).
			Where("conversation_id = ? AND status = ? AND resource_type = ? AND resource_name = ? AND namespace = ?",
				conversationID, "pending", params.Action, params.Name, params.Namespace).
			Update("status", "cancelled").Error
	}
	paramBytes, _ := json.Marshal(params)
	desc := strings.TrimSpace(description)
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
		return nil, "", fmt.Errorf("failed to stage action: %w", err)
	}
	pending := &PendingActionInfo{
		ID: rec.ID, ActionID: rec.ID, Action: params.Action,
		Name: params.Name, Namespace: params.Namespace,
		Description: desc, DryRun: dry, NeedConfirm: true,
	}
	return pending, dry, nil
}

func (s *Service) toolStageMutations(ctx context.Context, userID, clusterID, conversationID uint, argsJSON string) toolExecResult {
	var args struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := parseToolArgs(argsJSON, &args); err != nil {
		return toolExecResult{Content: err.Error(), IsError: true}
	}
	if len(args.Items) == 0 {
		return toolExecResult{Content: "items is required", IsError: true}
	}
	if len(args.Items) > 30 {
		return toolExecResult{Content: "too many items (max 30)", IsError: true}
	}
	var b strings.Builder
	var pendings []PendingActionInfo
	okCount := 0
	for i, raw := range args.Items {
		params, err := parseMutationParams(string(raw))
		if err != nil {
			b.WriteString(fmt.Sprintf("[%d] error: %s\n", i, err.Error()))
			continue
		}
		var meta struct {
			Description string `json:"description"`
		}
		_ = parseToolArgs(string(raw), &meta)
		if err := s.ensureToolAccess(ctx, userID, clusterID, params.Namespace, true); err != nil {
			b.WriteString(fmt.Sprintf("[%d] denied %s/%s: %s\n", i, params.Namespace, params.Name, err.Error()))
			continue
		}
		pending, dry, err := s.stageOneMutation(ctx, userID, clusterID, conversationID, params, meta.Description)
		if err != nil {
			b.WriteString(fmt.Sprintf("[%d] error %s/%s: %s\n", i, params.Namespace, params.Name, err.Error()))
			continue
		}
		okCount++
		pendings = append(pendings, *pending)
		b.WriteString(fmt.Sprintf("[%d] staged action_id=%d\n%s\n", i, pending.ID, dry))
	}
	b.WriteString(fmt.Sprintf("staged_ok=%d total=%d; confirm in UI\n", okCount, len(args.Items)))
	res := toolExecResult{Content: truncateRunes(b.String(), toolResultMaxChars), PendingList: pendings}
	if okCount == 0 {
		res.IsError = true
	} else if len(pendings) > 0 {
		p := pendings[len(pendings)-1]
		res.Pending = &p
	}
	return res
}

func (s *Service) toolDeleteByPrefix(ctx context.Context, userID, clusterID, conversationID uint, argsJSON string) toolExecResult {
	var args struct {
		Namespace  string `json:"namespace"`
		NamePrefix string `json:"name_prefix"`
		Limit      int    `json:"limit"`
	}
	if err := parseToolArgs(argsJSON, &args); err != nil {
		return toolExecResult{Content: err.Error(), IsError: true}
	}
	ns := strings.TrimSpace(args.Namespace)
	prefix := strings.TrimSpace(args.NamePrefix)
	if ns == "" || prefix == "" {
		return toolExecResult{Content: "namespace and name_prefix are required", IsError: true}
	}
	limit := args.Limit
	if limit <= 0 || limit > 30 {
		limit = 20
	}
	listArgs, _ := json.Marshal(map[string]interface{}{
		"resource_type": "pods", "namespace": ns, "name_prefix": prefix, "limit": limit,
	})
	listed := s.toolListResources(ctx, clusterID, string(listArgs))
	if listed.IsError {
		return listed
	}
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return toolExecResult{Content: err.Error(), IsError: true}
	}
	pods, err := client.Clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return toolExecResult{Content: err.Error(), IsError: true}
	}
	var b strings.Builder
	var pendings []PendingActionInfo
	n := 0
	for _, p := range pods.Items {
		if !strings.HasPrefix(p.Name, prefix) {
			continue
		}
		if n >= limit {
			b.WriteString(fmt.Sprintf("truncated staging at limit=%d\n", limit))
			break
		}
		params := StagedActionParams{Action: "delete_pod", Namespace: ns, Name: p.Name}
		pending, dry, err := s.stageOneMutation(ctx, userID, clusterID, conversationID, params, "delete_by_prefix "+prefix)
		if err != nil {
			b.WriteString(fmt.Sprintf("skip %s: %s\n", p.Name, err.Error()))
			continue
		}
		n++
		pendings = append(pendings, *pending)
		b.WriteString(fmt.Sprintf("staged action_id=%d %s\n", pending.ID, dry))
	}
	if n == 0 {
		return toolExecResult{Content: fmt.Sprintf("no pods matched prefix %q in ns %q\n%s", prefix, ns, listed.Content), IsError: true}
	}
	b.WriteString(fmt.Sprintf("staged_delete_count=%d prefix=%q ns=%q; confirm in UI\n", n, prefix, ns))
	res := toolExecResult{Content: truncateRunes(b.String(), toolResultMaxChars), PendingList: pendings}
	if len(pendings) > 0 {
		p := pendings[len(pendings)-1]
		res.Pending = &p
	}
	return res
}

func (s *Service) toolDiagnoseWorkload(ctx context.Context, clusterID uint, argsJSON string) toolExecResult {
	var args struct {
		ResourceType string `json:"resource_type"`
		Namespace    string `json:"namespace"`
		Name         string `json:"name"`
		TailLines    int64  `json:"tail_lines"`
	}
	if err := parseToolArgs(argsJSON, &args); err != nil {
		return toolExecResult{Content: err.Error(), IsError: true}
	}
	rt := strings.ToLower(strings.TrimSpace(args.ResourceType))
	if args.Namespace == "" || args.Name == "" {
		return toolExecResult{Content: "namespace and name are required", IsError: true}
	}
	var b strings.Builder
	b.WriteString("=== diagnose pipeline ===\n")

	getArgs, _ := json.Marshal(map[string]string{"resource_type": rt, "namespace": args.Namespace, "name": args.Name})
	getRes := s.toolGetResource(ctx, clusterID, string(getArgs))
	b.WriteString("--- get_resource ---\n")
	b.WriteString(getRes.Content)
	b.WriteString("\n")

	evArgs, _ := json.Marshal(map[string]interface{}{"namespace": args.Namespace, "name": args.Name, "limit": 20})
	evRes := s.toolGetEvents(ctx, clusterID, string(evArgs))
	b.WriteString("--- events ---\n")
	b.WriteString(evRes.Content)
	b.WriteString("\n")

	if rt == "pod" || rt == "pods" {
		tail := args.TailLines
		if tail <= 0 {
			tail = 80
		}
		logArgs, _ := json.Marshal(map[string]interface{}{
			"namespace": args.Namespace, "name": args.Name, "tail_lines": tail,
		})
		logRes := s.toolGetPodLogs(ctx, clusterID, string(logArgs))
		b.WriteString("--- logs ---\n")
		b.WriteString(logRes.Content)
		b.WriteString("\n")
	} else if rt == "deployment" || rt == "deployments" || rt == "deploy" {
		// List pods owned by deployment via label app=<name> heuristic + name prefix
		listArgs, _ := json.Marshal(map[string]interface{}{
			"resource_type": "pods", "namespace": args.Namespace, "name_prefix": args.Name, "limit": 10,
		})
		listRes := s.toolListResources(ctx, clusterID, string(listArgs))
		b.WriteString("--- related pods ---\n")
		b.WriteString(listRes.Content)
		b.WriteString("\n")
	}

	descArgs, _ := json.Marshal(map[string]string{"resource_type": rt, "namespace": args.Namespace, "name": args.Name})
	descRes := s.toolDescribeResource(ctx, clusterID, string(descArgs))
	b.WriteString("--- describe ---\n")
	b.WriteString(descRes.Content)

	return toolExecResult{Content: truncateRunes(b.String(), toolResultMaxChars)}
}

func formatServicePorts(ports []corev1.ServicePort) string {
	if len(ports) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ports))
	for _, p := range ports {
		name := p.Name
		if name == "" {
			name = "port"
		}
		part := fmt.Sprintf("%s %d->%s", name, p.Port, p.TargetPort.String())
		if p.NodePort > 0 {
			part += fmt.Sprintf(" nodePort=%d", p.NodePort)
		}
		part += "/" + string(p.Protocol)
		parts = append(parts, part)
	}
	return strings.Join(parts, "; ")
}

func (s *Service) summarizeService(ctx context.Context, client *k8s.ClusterClient, svc *corev1.Service) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("service %s/%s type=%s clusterIP=%s\n", svc.Namespace, svc.Name, svc.Spec.Type, svc.Spec.ClusterIP))
	b.WriteString(fmt.Sprintf("ports: [%s]\n", formatServicePorts(svc.Spec.Ports)))
	b.WriteString(fmt.Sprintf("selector: %v\n", svc.Spec.Selector))

	ready, notReady := 0, 0
	var addrs []string
	eps, err := client.Clientset.CoreV1().Endpoints(svc.Namespace).Get(ctx, svc.Name, metav1.GetOptions{})
	if err != nil {
		b.WriteString(fmt.Sprintf("endpoints: error=%v\n", err))
	} else {
		for _, subset := range eps.Subsets {
			ready += len(subset.Addresses)
			notReady += len(subset.NotReadyAddresses)
			for _, a := range subset.Addresses {
				addrs = append(addrs, a.IP)
			}
		}
		b.WriteString(fmt.Sprintf("endpoints: ready=%d notReady=%d addresses=%v\n", ready, notReady, addrs))
	}

	if len(svc.Spec.Selector) == 0 {
		b.WriteString("diagnosis: Service has empty selector (ExternalName or manually managed Endpoints).\n")
		return b.String()
	}

	sel := labels.Set(svc.Spec.Selector).AsSelector().String()
	var matchedPods []corev1.Pod
	pods, err := client.Clientset.CoreV1().Pods(svc.Namespace).List(ctx, metav1.ListOptions{LabelSelector: sel})
	if err != nil {
		b.WriteString(fmt.Sprintf("pods_matching_selector: error=%v selector=%q\n", err, sel))
	} else {
		matchedPods = pods.Items
		b.WriteString(fmt.Sprintf("pods_matching_selector: count=%d selector=%q\n", len(matchedPods), sel))
		for _, p := range matchedPods {
			b.WriteString(fmt.Sprintf("  - %s phase=%s ready=%v labels=%v containerPorts=%s\n",
				p.Name, p.Status.Phase, podReady(&p), p.Labels, formatContainerPorts(&p)))
		}
	}

	if ready == 0 {
		b.WriteString("diagnosis: Endpoints empty — Service/NodePort has no backend; do NOT blame firewall first.\n")
		b.WriteString(s.analyzeSelectorNearMiss(ctx, client, svc))
	} else {
		b.WriteString("diagnosis: Endpoints present — if still unreachable, verify client hits a Ready node IP and that nodePort is allowed by host firewall/security group.\n")
		b.WriteString(s.analyzeTargetPortCoverage(svc, matchedPods))
	}
	return b.String()
}

type selectorScore struct {
	Kind        string
	Name        string
	Exact       int
	Mismatched  []string
	MissingKeys []string
	Labels      map[string]string
}

func scoreAgainstSelector(want, got map[string]string) (exact int, mismatched, missing []string) {
	for k, v := range want {
		gv, ok := got[k]
		if !ok {
			missing = append(missing, k)
			continue
		}
		if gv == v {
			exact++
		} else {
			mismatched = append(mismatched, fmt.Sprintf("%s: service=%q workload=%q", k, v, gv))
		}
	}
	return
}

func (s *Service) analyzeSelectorNearMiss(ctx context.Context, client *k8s.ClusterClient, svc *corev1.Service) string {
	var b strings.Builder
	b.WriteString("--- selector_near_miss (same namespace, ranked) ---\n")

	var scores []selectorScore
	pods, err := client.Clientset.CoreV1().Pods(svc.Namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for i := range pods.Items {
			p := &pods.Items[i]
			exact, mismatched, missing := scoreAgainstSelector(svc.Spec.Selector, p.Labels)
			if exact == 0 && len(mismatched) == 0 {
				continue // no shared keys — low signal
			}
			scores = append(scores, selectorScore{
				Kind: "pod", Name: p.Name, Exact: exact, Mismatched: mismatched, MissingKeys: missing, Labels: p.Labels,
			})
		}
	}
	deploys, err := client.Clientset.AppsV1().Deployments(svc.Namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for i := range deploys.Items {
			d := &deploys.Items[i]
			lbl := d.Spec.Selector.MatchLabels
			if len(lbl) == 0 && d.Spec.Template.Labels != nil {
				lbl = d.Spec.Template.Labels
			}
			exact, mismatched, missing := scoreAgainstSelector(svc.Spec.Selector, lbl)
			if exact == 0 && len(mismatched) == 0 {
				continue
			}
			scores = append(scores, selectorScore{
				Kind: "deployment", Name: d.Name, Exact: exact, Mismatched: mismatched, MissingKeys: missing, Labels: lbl,
			})
		}
	}

	sort.SliceStable(scores, func(i, j int) bool {
		if scores[i].Exact != scores[j].Exact {
			return scores[i].Exact > scores[j].Exact
		}
		// Prefer key-value mismatches (same key, different value) over only missing keys.
		if len(scores[i].Mismatched) != len(scores[j].Mismatched) {
			return len(scores[i].Mismatched) > len(scores[j].Mismatched)
		}
		return scores[i].Name < scores[j].Name
	})

	limit := 8
	if len(scores) == 0 {
		b.WriteString("no workloads share any selector keys with this Service; check wrong namespace or Service selector empty/wrong.\n")
		return b.String()
	}
	if len(scores) > limit {
		scores = scores[:limit]
	}
	for _, sc := range scores {
		b.WriteString(fmt.Sprintf("- %s/%s exact=%d/%d mismatched=%v missing_keys=%v labels=%v\n",
			sc.Kind, sc.Name, sc.Exact, len(svc.Spec.Selector), sc.Mismatched, sc.MissingKeys, sc.Labels))
	}
	// Actionable top candidate
	top := scores[0]
	if len(top.Mismatched) > 0 {
		b.WriteString(fmt.Sprintf("likely_fix: align Service.selector with %s/%s labels (or vice versa). Diff: %v\n",
			top.Kind, top.Name, top.Mismatched))
	} else if top.Exact < len(svc.Spec.Selector) {
		b.WriteString(fmt.Sprintf("likely_fix: %s/%s is missing selector keys %v required by Service.\n",
			top.Kind, top.Name, top.MissingKeys))
	}
	return b.String()
}

func formatContainerPorts(p *corev1.Pod) string {
	var parts []string
	for _, c := range p.Spec.Containers {
		for _, cp := range c.Ports {
			parts = append(parts, fmt.Sprintf("%s:%d/%s", c.Name, cp.ContainerPort, cp.Protocol))
		}
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ",")
}

func (s *Service) analyzeTargetPortCoverage(svc *corev1.Service, pods []corev1.Pod) string {
	if len(pods) == 0 || len(svc.Spec.Ports) == 0 {
		return ""
	}
	var b strings.Builder
	for _, sp := range svc.Spec.Ports {
		tp := sp.TargetPort
		okAny := false
		for i := range pods {
			for _, c := range pods[i].Spec.Containers {
				for _, cp := range c.Ports {
					if tp.Type == intstr.String && cp.Name == tp.StrVal {
						okAny = true
					}
					if tp.Type == intstr.Int && cp.ContainerPort == tp.IntVal {
						okAny = true
					}
				}
			}
		}
		// targetPort may be numeric without being declared in container ports — still often works.
		if !okAny && tp.Type == intstr.Int {
			b.WriteString(fmt.Sprintf("note: targetPort=%s not declared on matching pod containerPorts (may still work if process listens).\n", tp.String()))
		}
	}
	return b.String()
}

func podReady(p *corev1.Pod) bool {
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

func (s *Service) toolDiagnoseService(ctx context.Context, clusterID uint, argsJSON string) toolExecResult {
	var args struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
		NodePort  int32  `json:"node_port"`
	}
	if err := parseToolArgs(argsJSON, &args); err != nil {
		return toolExecResult{Content: err.Error(), IsError: true}
	}
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return toolExecResult{Content: err.Error(), IsError: true}
	}

	ns := strings.TrimSpace(args.Namespace)
	name := strings.TrimSpace(args.Name)
	var b strings.Builder
	b.WriteString("=== diagnose_service pipeline ===\n")

	// Resolve by nodePort when name omitted — works for any Service, not a fixed app.
	if name == "" && args.NodePort > 0 {
		if ns == "" {
			ns = "default"
		}
		list, err := client.Clientset.CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			return toolExecResult{Content: err.Error(), IsError: true}
		}
		var hits []string
		for _, svc := range list.Items {
			for _, p := range svc.Spec.Ports {
				if p.NodePort == args.NodePort {
					hits = append(hits, svc.Name)
					if name == "" {
						name = svc.Name
					}
				}
			}
		}
		b.WriteString(fmt.Sprintf("resolve_by_nodePort=%d namespace=%s hits=%v\n", args.NodePort, ns, hits))
		if name == "" {
			// Broaden: search other namespaces the API can see (still general).
			allNS, err := client.Clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
			if err == nil {
				for _, nsi := range allNS.Items {
					if nsi.Name == ns {
						continue
					}
					svcs, err := client.Clientset.CoreV1().Services(nsi.Name).List(ctx, metav1.ListOptions{})
					if err != nil {
						continue
					}
					for _, svc := range svcs.Items {
						for _, p := range svc.Spec.Ports {
							if p.NodePort == args.NodePort {
								hits = append(hits, nsi.Name+"/"+svc.Name)
								if name == "" {
									ns = nsi.Name
									name = svc.Name
								}
							}
						}
					}
				}
			}
			b.WriteString(fmt.Sprintf("cluster_search_hits=%v\n", hits))
		}
		if name == "" {
			b.WriteString(fmt.Sprintf("no Service exposes nodePort=%d\n", args.NodePort))
			return toolExecResult{Content: b.String(), IsError: true}
		}
	}

	if ns == "" || name == "" {
		return toolExecResult{Content: "namespace+name required, or provide node_port (optional namespace, default=default)", IsError: true}
	}

	getArgs, _ := json.Marshal(map[string]string{
		"resource_type": "service", "namespace": ns, "name": name,
	})
	res := s.toolGetResource(ctx, clusterID, string(getArgs))
	if res.IsError {
		return toolExecResult{Content: b.String() + res.Content, IsError: true}
	}
	b.WriteString(res.Content)
	b.WriteString("\n--- events ---\n")
	evArgs, _ := json.Marshal(map[string]interface{}{"namespace": ns, "name": name, "limit": 15})
	b.WriteString(s.toolGetEvents(ctx, clusterID, string(evArgs)).Content)
	return toolExecResult{Content: truncateRunes(b.String(), toolResultMaxChars)}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
