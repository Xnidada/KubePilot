package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cache 缓存接口
type Cache interface {
	// Get 获取缓存
	Get(ctx context.Context, key string) (string, error)
	// Set 设置缓存
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error
	// Delete 删除缓存
	Delete(ctx context.Context, key string) error
	// Take 原子获取并删除缓存，用于一次性令牌
	Take(ctx context.Context, key string) (string, error)
	// Exists 检查缓存是否存在
	Exists(ctx context.Context, key string) (bool, error)
	// SetNX 仅在 key 不存在时设置
	SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error)
	// Increment 递增
	Increment(ctx context.Context, key string) (int64, error)
	// GetTTL 获取剩余过期时间
	GetTTL(ctx context.Context, key string) (time.Duration, error)
	// Close 关闭连接
	Close() error
}

// ==================== 内存缓存实现 ====================

// MemoryCache 内存缓存
type MemoryCache struct {
	mu     sync.RWMutex
	items  map[string]*cacheItem
	stopCh chan struct{}
}

type cacheItem struct {
	Value      string
	Expiration int64 // Unix timestamp, 0 表示永不过期
	CreatedAt  time.Time
}

// NewMemoryCache 创建内存缓存
func NewMemoryCache() *MemoryCache {
	mc := &MemoryCache{
		items:  make(map[string]*cacheItem),
		stopCh: make(chan struct{}),
	}
	// 启动清理协程
	go mc.cleanup()
	return mc
}

func (mc *MemoryCache) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			mc.deleteExpired()
		case <-mc.stopCh:
			return
		}
	}
}

func (mc *MemoryCache) deleteExpired() {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	now := time.Now().Unix()
	for k, v := range mc.items {
		if v.Expiration > 0 && v.Expiration <= now {
			delete(mc.items, k)
		}
	}
}

func (mc *MemoryCache) Get(ctx context.Context, key string) (string, error) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	item, ok := mc.items[key]
	if !ok {
		return "", fmt.Errorf("key not found: %s", key)
	}

	if item.Expiration > 0 && item.Expiration <= time.Now().Unix() {
		return "", fmt.Errorf("key expired: %s", key)
	}

	return item.Value, nil
}

func (mc *MemoryCache) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	var strValue string
	switch v := value.(type) {
	case string:
		strValue = v
	case []byte:
		strValue = string(v)
	default:
		jsonBytes, err := json.Marshal(value)
		if err != nil {
			return err
		}
		strValue = string(jsonBytes)
	}

	var exp int64
	if expiration > 0 {
		exp = time.Now().Add(expiration).Unix()
	}

	mc.items[key] = &cacheItem{
		Value:      strValue,
		Expiration: exp,
		CreatedAt:  time.Now(),
	}

	return nil
}

func (mc *MemoryCache) Delete(ctx context.Context, key string) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	delete(mc.items, key)
	return nil
}

func (mc *MemoryCache) Take(ctx context.Context, key string) (string, error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	item, ok := mc.items[key]
	if !ok {
		return "", fmt.Errorf("key not found: %s", key)
	}
	delete(mc.items, key)
	if item.Expiration > 0 && item.Expiration <= time.Now().Unix() {
		return "", fmt.Errorf("key expired: %s", key)
	}
	return item.Value, nil
}

func (mc *MemoryCache) Exists(ctx context.Context, key string) (bool, error) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	item, ok := mc.items[key]
	if !ok {
		return false, nil
	}

	if item.Expiration > 0 && item.Expiration <= time.Now().Unix() {
		return false, nil
	}

	return true, nil
}

func (mc *MemoryCache) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	item, ok := mc.items[key]
	if ok && (item.Expiration == 0 || item.Expiration > time.Now().Unix()) {
		return false, nil
	}

	var strValue string
	switch v := value.(type) {
	case string:
		strValue = v
	case []byte:
		strValue = string(v)
	default:
		jsonBytes, err := json.Marshal(value)
		if err != nil {
			return false, err
		}
		strValue = string(jsonBytes)
	}

	var exp int64
	if expiration > 0 {
		exp = time.Now().Add(expiration).Unix()
	}

	mc.items[key] = &cacheItem{
		Value:      strValue,
		Expiration: exp,
		CreatedAt:  time.Now(),
	}

	return true, nil
}

func (mc *MemoryCache) Increment(ctx context.Context, key string) (int64, error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	item, ok := mc.items[key]
	if !ok {
		mc.items[key] = &cacheItem{
			Value:     "1",
			CreatedAt: time.Now(),
		}
		return 1, nil
	}

	var val int64
	fmt.Sscanf(item.Value, "%d", &val)
	val++
	item.Value = fmt.Sprintf("%d", val)

	return val, nil
}

func (mc *MemoryCache) GetTTL(ctx context.Context, key string) (time.Duration, error) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	item, ok := mc.items[key]
	if !ok {
		return 0, fmt.Errorf("key not found: %s", key)
	}

	if item.Expiration == 0 {
		return -1, nil // 永不过期
	}

	remaining := time.Until(time.Unix(item.Expiration, 0))
	if remaining <= 0 {
		return 0, nil
	}

	return remaining, nil
}

func (mc *MemoryCache) Close() error {
	close(mc.stopCh)
	return nil
}

// ==================== Redis 缓存实现 ====================

type RedisCache struct {
	client *redis.Client
}

func NewRedisCache(addr, password string, db int) *RedisCache {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	return &RedisCache{client: client}
}

func (rc *RedisCache) Get(ctx context.Context, key string) (string, error) {
	return rc.client.Get(ctx, key).Result()
}

func (rc *RedisCache) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	serialized, err := serializeValue(value)
	if err != nil {
		return err
	}
	return rc.client.Set(ctx, key, serialized, expiration).Err()
}

func (rc *RedisCache) Delete(ctx context.Context, key string) error {
	return rc.client.Del(ctx, key).Err()
}

func (rc *RedisCache) Take(ctx context.Context, key string) (string, error) {
	return rc.client.GetDel(ctx, key).Result()
}

func (rc *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	count, err := rc.client.Exists(ctx, key).Result()
	return count > 0, err
}

func (rc *RedisCache) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error) {
	serialized, err := serializeValue(value)
	if err != nil {
		return false, err
	}
	return rc.client.SetNX(ctx, key, serialized, expiration).Result()
}

func (rc *RedisCache) Increment(ctx context.Context, key string) (int64, error) {
	return rc.client.Incr(ctx, key).Result()
}

func (rc *RedisCache) GetTTL(ctx context.Context, key string) (time.Duration, error) {
	return rc.client.TTL(ctx, key).Result()
}

func (rc *RedisCache) Close() error {
	return rc.client.Close()
}

func serializeValue(value interface{}) (interface{}, error) {
	switch value := value.(type) {
	case string:
		return value, nil
	case []byte:
		return value, nil
	default:
		return json.Marshal(value)
	}
}

// ==================== 工厂函数 ====================

// Config 缓存配置
type Config struct {
	Type     string // "memory" 或 "redis"
	Addr     string
	Password string
	DB       int
}

// New 创建缓存实例并验证外部缓存连接。
func New(cfg Config) (Cache, error) {
	switch cfg.Type {
	case "redis":
		redisCache := NewRedisCache(cfg.Addr, cfg.Password, cfg.DB)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := redisCache.client.Ping(ctx).Err(); err != nil {
			_ = redisCache.Close()
			return nil, fmt.Errorf("connect to redis at %s: %w", cfg.Addr, err)
		}
		return redisCache, nil
	default:
		return NewMemoryCache(), nil
	}
}
