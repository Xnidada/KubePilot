package netutil

import (
	"net"
	"strings"

	"github.com/gin-gonic/gin"
)

// RealClientIP extracts the real client IP from the request.
// It checks X-Real-IP and X-Forwarded-For headers (set by nginx/ingress
// or other reverse proxies) before falling back to Gin's ClientIP().
//
// Priority:
//  1. X-Real-IP header (nginx: proxy_set_header X-Real-IP $remote_addr)
//  2. First entry in X-Forwarded-For (original client IP)
//  3. Gin's ClientIP() (which respects TrustedProxies config)
func RealClientIP(c *gin.Context) string {
	// 1. X-Real-IP
	if xrip := c.GetHeader("X-Real-Ip"); xrip != "" {
		if ip := net.ParseIP(strings.TrimSpace(xrip)); ip != nil {
			return ip.String()
		}
	}
	// 2. X-Forwarded-For: client, proxy1, proxy2 — take the first (leftmost) entry
	if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		candidate := strings.TrimSpace(parts[0])
		if ip := net.ParseIP(candidate); ip != nil {
			return ip.String()
		}
	}
	// 3. Fallback to Gin's built-in (which respects TrustedProxies)
	return c.ClientIP()
}
