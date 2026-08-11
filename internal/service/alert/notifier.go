package alert

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/logger"
	"github.com/kubepilot/kubepilot/internal/pkg/netutil"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Notifier struct {
	db     *gorm.DB
	client *http.Client
}

func NewNotifier(db *gorm.DB) *Notifier {
	return &Notifier{
		db: db,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

type channelConfig struct {
	WebhookURL string `json:"webhook_url"`
	URL        string `json:"url"`
	Secret     string `json:"secret"`
}

// Notify 向指定渠道或所有启用渠道发送通知
func (n *Notifier) Notify(ctx context.Context, channelIDs []uint, title, message, status string, history *model.AlertHistory) error {
	channels, err := n.loadChannels(channelIDs)
	if err != nil {
		return err
	}
	if len(channels) == 0 {
		logger.Warn("no notification channels available")
		return nil
	}

	var lastErr error
	notified := false
	for _, ch := range channels {
		if err := n.send(ctx, &ch, title, message, status); err != nil {
			logger.Error("failed to send notification",
				zap.Uint("channel_id", ch.ID),
				zap.String("type", ch.Type),
				zap.Error(err),
			)
			lastErr = err
			continue
		}
		notified = true
	}

	if history != nil && history.ID > 0 && notified {
		now := time.Now()
		_ = n.db.Model(history).Updates(map[string]interface{}{
			"notified":  true,
			"notify_at": now,
		}).Error
	}

	return lastErr
}

func (n *Notifier) loadChannels(channelIDs []uint) ([]model.NotificationChannel, error) {
	var channels []model.NotificationChannel
	query := n.db.Where("enabled = ?", true)
	if len(channelIDs) > 0 {
		query = query.Where("id IN ?", channelIDs)
	}
	if err := query.Find(&channels).Error; err != nil {
		return nil, err
	}
	return channels, nil
}

func (n *Notifier) send(ctx context.Context, ch *model.NotificationChannel, title, message, status string) error {
	cfg, err := parseChannelConfig(ch.Config)
	if err != nil {
		return err
	}
	webhookURL := cfg.WebhookURL
	if webhookURL == "" {
		webhookURL = cfg.URL
	}
	if webhookURL == "" {
		return fmt.Errorf("channel %d missing webhook_url", ch.ID)
	}

	switch strings.ToLower(ch.Type) {
	case "dingtalk":
		return n.sendDingTalk(ctx, webhookURL, cfg.Secret, title, message)
	case "feishu", "lark":
		return n.sendFeishu(ctx, webhookURL, title, message)
	case "wechat", "wecom":
		return n.sendWeChat(ctx, webhookURL, title, message)
	case "webhook":
		fallthrough
	default:
		return n.sendWebhook(ctx, webhookURL, title, message, status)
	}
}

func parseChannelConfig(raw string) (*channelConfig, error) {
	cfg := &channelConfig{}
	if strings.TrimSpace(raw) == "" {
		return cfg, nil
	}
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		return nil, fmt.Errorf("invalid channel config: %w", err)
	}
	return cfg, nil
}

func (n *Notifier) sendWebhook(ctx context.Context, webhookURL, title, message, status string) error {
	payload := map[string]interface{}{
		"title":   title,
		"message": message,
		"status":  status,
		"source":  "kubepilot",
		"time":    time.Now().Format(time.RFC3339),
	}
	return n.postJSON(ctx, webhookURL, payload)
}

func (n *Notifier) sendDingTalk(ctx context.Context, webhookURL, secret, title, message string) error {
	finalURL := webhookURL
	if secret != "" {
		ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
		stringToSign := ts + "\n" + secret
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write([]byte(stringToSign))
		sign := url.QueryEscape(base64.StdEncoding.EncodeToString(mac.Sum(nil)))
		sep := "?"
		if strings.Contains(webhookURL, "?") {
			sep = "&"
		}
		finalURL = webhookURL + sep + "timestamp=" + ts + "&sign=" + sign
	}

	content := title
	if message != "" {
		content = title + "\n" + message
	}
	payload := map[string]interface{}{
		"msgtype": "text",
		"text": map[string]string{
			"content": content,
		},
	}
	return n.postJSON(ctx, finalURL, payload)
}

func (n *Notifier) sendFeishu(ctx context.Context, webhookURL, title, message string) error {
	content := title
	if message != "" {
		content = title + "\n" + message
	}
	payload := map[string]interface{}{
		"msg_type": "text",
		"content": map[string]string{
			"text": content,
		},
	}
	return n.postJSON(ctx, webhookURL, payload)
}

func (n *Notifier) sendWeChat(ctx context.Context, webhookURL, title, message string) error {
	content := title
	if message != "" {
		content = title + "\n" + message
	}
	payload := map[string]interface{}{
		"msgtype": "text",
		"text": map[string]string{
			"content": content,
		},
	}
	return n.postJSON(ctx, webhookURL, payload)
}

func (n *Notifier) postJSON(ctx context.Context, webhookURL string, payload interface{}) error {
	if err := netutil.ValidateOutboundURL(webhookURL); err != nil {
		return fmt.Errorf("ssrf blocked: %w", err)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// ParseChannelIDs 解析规则中的 channels JSON
func ParseChannelIDs(raw string) []uint {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var ids []uint
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil
	}
	return ids
}
