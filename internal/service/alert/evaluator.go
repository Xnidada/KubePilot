package alert

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	DefaultInterval = 60 * time.Second
	CooldownPeriod  = 5 * time.Minute
)

type breachKey struct {
	RuleID    uint
	Namespace string
	Resource  string
}

type Evaluator struct {
	db       *gorm.DB
	notifier *Notifier
	interval time.Duration

	mu      sync.Mutex
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	pending map[breachKey]time.Time
}

func NewEvaluator(db *gorm.DB, interval time.Duration) *Evaluator {
	if interval <= 0 {
		interval = DefaultInterval
	}
	return &Evaluator{
		db:       db,
		notifier: NewNotifier(db),
		interval: interval,
		pending:  make(map[breachKey]time.Time),
	}
}

func (e *Evaluator) Start() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		e.loop(ctx)
	}()
	logger.Info("alert evaluator started", zap.Duration("interval", e.interval))
}

func (e *Evaluator) Stop() {
	e.mu.Lock()
	cancel := e.cancel
	e.cancel = nil
	e.mu.Unlock()
	if cancel != nil {
		cancel()
		e.wg.Wait()
		logger.Info("alert evaluator stopped")
	}
}

func (e *Evaluator) loop(ctx context.Context) {
	// 启动后先跑一轮
	e.EvaluateAll(ctx)

	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.EvaluateAll(ctx)
		}
	}
}

func (e *Evaluator) EvaluateAll(ctx context.Context) {
	var rules []model.AlertRule
	if err := e.db.Where("enabled = ?", true).Find(&rules).Error; err != nil {
		logger.Error("failed to load alert rules", zap.Error(err))
		return
	}
	for i := range rules {
		if ctx.Err() != nil {
			return
		}
		e.evaluateRule(ctx, &rules[i])
	}
}

type metricSample struct {
	Namespace string
	Resource  string
	Value     float64
}

func (e *Evaluator) markEval(rule *model.AlertRule, evalErr error) {
	now := time.Now()
	updates := map[string]interface{}{
		"last_eval_at": now,
	}
	if evalErr != nil {
		msg := evalErr.Error()
		if len(msg) > 500 {
			msg = msg[:500]
		}
		updates["last_eval_error"] = msg
	} else {
		updates["last_eval_error"] = ""
	}
	_ = e.db.Model(rule).Updates(updates).Error
	rule.LastEvalAt = &now
	if evalErr != nil {
		rule.LastEvalError = updates["last_eval_error"].(string)
	} else {
		rule.LastEvalError = ""
	}
}

func parseDuration(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "0" || raw == "0s" || raw == "0m" {
		return 0, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: %w", raw, err)
	}
	if d < 0 {
		return 0, fmt.Errorf("duration must be non-negative")
	}
	return d, nil
}

func (e *Evaluator) evaluateRule(ctx context.Context, rule *model.AlertRule) {
	if k8s.Manager == nil {
		e.markEval(rule, fmt.Errorf("k8s manager not initialized"))
		logger.Warn("k8s manager not initialized, skip alert evaluation")
		return
	}

	duration, err := parseDuration(rule.Duration)
	if err != nil {
		e.markEval(rule, err)
		logger.Warn("invalid alert rule duration",
			zap.Uint("rule_id", rule.ID),
			zap.String("duration", rule.Duration),
			zap.Error(err),
		)
		return
	}

	client, err := k8s.Manager.GetClient(rule.ClusterID)
	if err != nil {
		e.markEval(rule, fmt.Errorf("cluster client: %w", err))
		logger.Warn("failed to get cluster client for alert rule",
			zap.Uint("rule_id", rule.ID),
			zap.Uint("cluster_id", rule.ClusterID),
			zap.Error(err),
		)
		return
	}
	if client.MetricsClient == nil {
		e.markEval(rule, fmt.Errorf("metrics-server unavailable for cluster %d", rule.ClusterID))
		logger.Warn("metrics client unavailable, skip rule",
			zap.Uint("rule_id", rule.ID),
			zap.Uint("cluster_id", rule.ClusterID),
		)
		return
	}

	samples, err := e.collectSamples(ctx, client, rule)
	if err != nil {
		e.markEval(rule, err)
		logger.Error("failed to collect metrics for alert rule",
			zap.Uint("rule_id", rule.ID),
			zap.Error(err),
		)
		return
	}

	e.markEval(rule, nil)

	active := make(map[breachKey]struct{}, len(samples))
	now := time.Now()
	for _, sample := range samples {
		key := breachKey{RuleID: rule.ID, Namespace: sample.Namespace, Resource: sample.Resource}
		if !matchCondition(sample.Value, rule.Condition, rule.Threshold) {
			e.clearPending(key)
			continue
		}
		active[key] = struct{}{}
		if duration <= 0 {
			e.fire(ctx, rule, sample)
			continue
		}
		first, ok := e.getPending(key)
		if !ok {
			e.setPending(key, now)
			continue
		}
		if now.Sub(first) >= duration {
			e.fire(ctx, rule, sample)
		}
	}

	// Drop pending entries for this rule that no longer breach.
	e.mu.Lock()
	for key := range e.pending {
		if key.RuleID != rule.ID {
			continue
		}
		if _, ok := active[key]; !ok {
			delete(e.pending, key)
		}
	}
	e.mu.Unlock()
}

func (e *Evaluator) getPending(key breachKey) (time.Time, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	t, ok := e.pending[key]
	return t, ok
}

func (e *Evaluator) setPending(key breachKey, t time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.pending[key] = t
}

func (e *Evaluator) clearPending(key breachKey) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.pending, key)
}

func (e *Evaluator) collectSamples(ctx context.Context, client *k8s.ClusterClient, rule *model.AlertRule) ([]metricSample, error) {
	resourceType := strings.ToLower(strings.TrimSpace(rule.Resource))
	// Resource 字段存的是类型（node/pod/deployment）或具体名称；优先按类型语义处理
	metric := strings.ToLower(strings.TrimSpace(rule.Metric))
	if metric != "cpu" && metric != "memory" {
		return nil, fmt.Errorf("unsupported metric %q", rule.Metric)
	}

	switch resourceType {
	case "", "node", "nodes":
		return e.collectNodeSamples(ctx, client, rule, metric)
	case "pod", "pods":
		if strings.TrimSpace(rule.Namespace) == "" {
			logger.Warn("pod alert rule missing namespace, skip", zap.Uint("rule_id", rule.ID))
			return nil, nil
		}
		return e.collectPodSamples(ctx, client, rule, metric, "")
	case "deployment", "deployments", "deploy":
		if strings.TrimSpace(rule.Namespace) == "" {
			logger.Warn("deployment alert rule missing namespace, skip", zap.Uint("rule_id", rule.ID))
			return nil, nil
		}
		return e.collectDeploymentSamples(ctx, client, rule, metric)
	default:
		// 兼容：resource 填的是具体资源名，需要结合命名空间判断
		if strings.TrimSpace(rule.Namespace) == "" {
			// 当作节点名
			return e.collectNodeSamples(ctx, client, rule, metric)
		}
		return e.collectPodSamples(ctx, client, rule, metric, resourceType)
	}
}

func (e *Evaluator) collectNodeSamples(ctx context.Context, client *k8s.ClusterClient, rule *model.AlertRule, metric string) ([]metricSample, error) {
	nodes, err := client.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	nodeMetrics, err := client.MetricsClient.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("metrics-server unavailable: %w", err)
	}
	usageByName := make(map[string]struct {
		cpuMilli int64
		memMi    int64
	}, len(nodeMetrics.Items))
	for _, nm := range nodeMetrics.Items {
		usageByName[nm.Name] = struct {
			cpuMilli int64
			memMi    int64
		}{
			cpuMilli: nm.Usage.Cpu().MilliValue(),
			memMi:    nm.Usage.Memory().Value() / (1024 * 1024),
		}
	}

	targetName := ""
	rt := strings.ToLower(strings.TrimSpace(rule.Resource))
	if rt != "" && rt != "node" && rt != "nodes" {
		targetName = rule.Resource
	}

	samples := make([]metricSample, 0)
	for _, node := range nodes.Items {
		if targetName != "" && node.Name != targetName {
			continue
		}
		usage, ok := usageByName[node.Name]
		if !ok {
			continue
		}
		var value float64
		switch metric {
		case "cpu":
			cap := node.Status.Capacity.Cpu().MilliValue()
			if cap <= 0 {
				continue
			}
			value = round2(float64(usage.cpuMilli) / float64(cap) * 100)
		case "memory":
			cap := node.Status.Capacity.Memory().Value() / (1024 * 1024)
			if cap <= 0 {
				continue
			}
			value = round2(float64(usage.memMi) / float64(cap) * 100)
		}
		samples = append(samples, metricSample{
			Resource: node.Name,
			Value:    value,
		})
	}
	return samples, nil
}

func (e *Evaluator) collectPodSamples(ctx context.Context, client *k8s.ClusterClient, rule *model.AlertRule, metric, podName string) ([]metricSample, error) {
	ns := rule.Namespace
	podMetrics, err := client.MetricsClient.MetricsV1beta1().PodMetricses(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("metrics-server unavailable: %w", err)
	}

	// 若未指定具体 pod 名，且 resource 不是类型关键字，则 resource 可能是 pod 名
	if podName == "" {
		rt := strings.ToLower(strings.TrimSpace(rule.Resource))
		if rt != "" && rt != "pod" && rt != "pods" {
			podName = rule.Resource
		}
	}

	samples := make([]metricSample, 0)
	for _, pm := range podMetrics.Items {
		if podName != "" && pm.Name != podName {
			continue
		}
		var cpuMilli, memMi int64
		for _, c := range pm.Containers {
			cpuMilli += c.Usage.Cpu().MilliValue()
			memMi += c.Usage.Memory().Value() / (1024 * 1024)
		}
		var value float64
		switch metric {
		case "cpu":
			value = float64(cpuMilli)
		case "memory":
			value = float64(memMi)
		}
		samples = append(samples, metricSample{
			Namespace: pm.Namespace,
			Resource:  pm.Name,
			Value:     value,
		})
	}
	return samples, nil
}

func (e *Evaluator) collectDeploymentSamples(ctx context.Context, client *k8s.ClusterClient, rule *model.AlertRule, metric string) ([]metricSample, error) {
	ns := rule.Namespace
	deployName := ""
	rt := strings.ToLower(strings.TrimSpace(rule.Resource))
	if rt != "" && rt != "deployment" && rt != "deployments" && rt != "deploy" {
		deployName = rule.Resource
	}

	deploys, err := client.Clientset.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	podMetrics, err := client.MetricsClient.MetricsV1beta1().PodMetricses(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("metrics-server unavailable: %w", err)
	}

	type agg struct {
		cpuMilli int64
		memMi    int64
	}
	byDeploy := map[string]*agg{}

	for _, d := range deploys.Items {
		if deployName != "" && d.Name != deployName {
			continue
		}
		byDeploy[d.Name] = &agg{}
		selector, err := metav1.LabelSelectorAsSelector(d.Spec.Selector)
		if err != nil {
			continue
		}
		for _, pm := range podMetrics.Items {
			if !selector.Matches(labels.Set(pm.Labels)) {
				continue
			}
			for _, c := range pm.Containers {
				byDeploy[d.Name].cpuMilli += c.Usage.Cpu().MilliValue()
				byDeploy[d.Name].memMi += c.Usage.Memory().Value() / (1024 * 1024)
			}
		}
	}

	samples := make([]metricSample, 0, len(byDeploy))
	for name, a := range byDeploy {
		var value float64
		switch metric {
		case "cpu":
			value = float64(a.cpuMilli)
		case "memory":
			value = float64(a.memMi)
		}
		samples = append(samples, metricSample{
			Namespace: ns,
			Resource:  name,
			Value:     value,
		})
	}
	return samples, nil
}

func (e *Evaluator) fire(ctx context.Context, rule *model.AlertRule, sample metricSample) {
	now := time.Now()
	if rule.LastAlert != nil && now.Sub(*rule.LastAlert) < CooldownPeriod {
		return
	}

	unit := metricUnit(rule)
	message := fmt.Sprintf("规则 %s 触发: %s %s %s %.2f%s (当前值 %.2f%s)",
		rule.Name, sample.Resource, rule.Metric, rule.Condition, rule.Threshold, unit, sample.Value, unit)
	if d := strings.TrimSpace(rule.Duration); d != "" && d != "0" && d != "0s" && d != "0m" {
		message += fmt.Sprintf(" [持续 %s]", d)
	}

	history := &model.AlertHistory{
		RuleID:      rule.ID,
		ClusterID:   rule.ClusterID,
		Namespace:   sample.Namespace,
		Resource:    sample.Resource,
		Message:     message,
		Value:       sample.Value,
		Status:      "firing",
		TriggeredAt: now,
	}
	if err := e.db.Create(history).Error; err != nil {
		logger.Error("failed to write alert history", zap.Error(err), zap.Uint("rule_id", rule.ID))
		return
	}

	_ = e.db.Model(rule).Update("last_alert", now).Error
	rule.LastAlert = &now

	channelIDs := ParseChannelIDs(rule.Channels)
	if len(channelIDs) == 0 {
		logger.Warn("alert rule has no channels, skip notify", zap.Uint("rule_id", rule.ID))
		return
	}
	title := fmt.Sprintf("[KubePilot] %s", rule.Name)
	if err := e.notifier.Notify(ctx, channelIDs, title, message, "firing", history); err != nil {
		logger.Warn("alert notification partially failed", zap.Uint("rule_id", rule.ID), zap.Error(err))
	}
}

func metricUnit(rule *model.AlertRule) string {
	rt := strings.ToLower(strings.TrimSpace(rule.Resource))
	isNode := rt == "" || rt == "node" || rt == "nodes" ||
		(strings.TrimSpace(rule.Namespace) == "" &&
			rt != "pod" && rt != "pods" &&
			rt != "deployment" && rt != "deployments" && rt != "deploy")
	switch strings.ToLower(rule.Metric) {
	case "cpu":
		if isNode {
			return "%"
		}
		return "m"
	case "memory":
		if isNode {
			return "%"
		}
		return "Mi"
	default:
		return ""
	}
}

func matchCondition(value float64, condition string, threshold float64) bool {
	switch strings.TrimSpace(condition) {
	case ">":
		return value > threshold
	case "<":
		return value < threshold
	case ">=":
		return value >= threshold
	case "<=":
		return value <= threshold
	case "==", "=":
		return math.Abs(value-threshold) < 1e-9
	case "!=":
		return math.Abs(value-threshold) >= 1e-9
	default:
		return false
	}
}

func round2(f float64) float64 {
	return math.Round(f*100) / 100
}
