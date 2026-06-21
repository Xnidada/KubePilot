package middleware

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/pkg/cache"
)

// RateLimiter 速率限制器
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rate     int           // 允许的请求数
	window   time.Duration // 时间窗口
	cache    cache.Cache   // 缓存实例（支持 Redis）
	useCache  bool
}

type visitor struct {
	count    int
	lastSeen time.Time
}

// NewRateLimiter 创建速率限制器
func NewRateLimiter(rate int, window time.Duration, cacheInstance ...cache.Cache) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     rate,
		window:   window,
	}

	// 如果提供了缓存实例，使用缓存
	if len(cacheInstance) > 0 && cacheInstance[0] != nil {
		rl.cache = cacheInstance[0]
		rl.useCache = true
	} else {
		// 否则使用内存，启动清理协程
		go rl.cleanup()
	}

	return rl
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		for ip, v := range rl.visitors {
			if time.Since(v.lastSeen) > rl.window {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *RateLimiter) Allow(ip string) bool {
	// 使用缓存实现
	if rl.useCache {
		return rl.allowWithCache(ip)
	}
	// 使用内存实现
	return rl.allowWithMemory(ip)
}

func (rl *RateLimiter) allowWithCache(ip string) bool {
	ctx := context.Background()
	key := fmt.Sprintf("ratelimit:%s", ip)

	// 获取当前计数
	countStr, err := rl.cache.Get(ctx, key)
	if err != nil {
		// key 不存在，设置为 1
		rl.cache.Set(ctx, key, "1", rl.window)
		return true
	}

	var count int
	fmt.Sscanf(countStr, "%d", &count)

	if count >= rl.rate {
		return false
	}

	// 递增计数
	rl.cache.Increment(ctx, key)
	return true
}

func (rl *RateLimiter) allowWithMemory(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, exists := rl.visitors[ip]
	if !exists {
		rl.visitors[ip] = &visitor{count: 1, lastSeen: time.Now()}
		return true
	}

	if time.Since(v.lastSeen) > rl.window {
		v.count = 1
		v.lastSeen = time.Now()
		return true
	}

	if v.count >= rl.rate {
		return false
	}

	v.count++
	v.lastSeen = time.Now()
	return true
}

// RateLimitMiddleware 创建速率限制中间件
func RateLimitMiddleware(rate int, window time.Duration, cacheInstance ...cache.Cache) gin.HandlerFunc {
	limiter := NewRateLimiter(rate, window, cacheInstance...)

	return func(c *gin.Context) {
		ip := c.ClientIP()

		if !limiter.Allow(ip) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"code":    429,
				"message": "too many requests, please try again later",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
