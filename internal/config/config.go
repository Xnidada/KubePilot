package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig             `mapstructure:"server"`
	Database DatabaseConfig           `mapstructure:"database"`
	Redis    RedisConfig              `mapstructure:"redis"`
	JWT      JWTConfig                `mapstructure:"jwt"`
	Security SecurityConfig           `mapstructure:"security"`
	Log      LogConfig                `mapstructure:"log"`
	K8S      K8SConfig                `mapstructure:"k8s"`
	LLM      LLMConfig                `mapstructure:"llm"`
	Cache    CacheConfig              `mapstructure:"cache"`
	Modules  map[string]ModuleConfig  `mapstructure:"modules"`
}

// ModuleConfig controls in-process feature modules.
type ModuleConfig struct {
	Enabled *bool `mapstructure:"enabled"`

	// Optional health tuning (used by modules that support it, e.g. eventforward).
	FailRateThreshold    float64 `mapstructure:"fail_rate_threshold"`     // 0-1, default 0.9
	MinMatched           int64   `mapstructure:"min_matched"`             // default 20
	HealthSustain        string  `mapstructure:"health_sustain"`          // e.g. "2m"; must stay bad this long before unhealthy
	DisableFailRateCheck *bool   `mapstructure:"disable_fail_rate_check"` // skip fail-rate based health
}

type ServerConfig struct {
	Host         string        `mapstructure:"host"`
	Port         int           `mapstructure:"port"`
	Mode         string        `mapstructure:"mode"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
}

type DatabaseConfig struct {
	Driver       string `mapstructure:"driver"` // postgres
	Host         string `mapstructure:"host"`
	Port         int    `mapstructure:"port"`
	Username     string `mapstructure:"username"`
	Password     string `mapstructure:"password"`
	DBName       string `mapstructure:"dbname"`
	SSLMode      string `mapstructure:"sslmode"`
	MaxIdleConns int    `mapstructure:"max_idle_conns"`
	MaxOpenConns int    `mapstructure:"max_open_conns"`
}

type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type JWTConfig struct {
	Secret     string        `mapstructure:"secret"`
	ExpireTime time.Duration `mapstructure:"expire_time"`
	Issuer     string        `mapstructure:"issuer"`
}

// SecurityConfig holds secrets that must remain stable across JWT rotations.
// Kubeconfig ciphertext in the database is sealed with EncryptKey; changing
// JWT.Secret alone must not invalidate cluster credentials.
type SecurityConfig struct {
	EncryptKey string `mapstructure:"encrypt_key"`
}

type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
	Output string `mapstructure:"output"`
}

type K8SConfig struct {
	DefaultNamespace string `mapstructure:"default_namespace"`
	QPS              float32 `mapstructure:"qps"`
	Burst            int     `mapstructure:"burst"`
}

type LLMConfig struct {
	Provider    string  `mapstructure:"provider"`    // openai, anthropic
	APIKey      string  `mapstructure:"api_key"`
	BaseURL     string  `mapstructure:"base_url"`
	Model       string  `mapstructure:"model"`
	Temperature float64 `mapstructure:"temperature"`
	MaxTokens   int     `mapstructure:"max_tokens"`
	Timeout     int     `mapstructure:"timeout"`
}

type CacheConfig struct {
	Type     string `mapstructure:"type"` // memory, redis
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./configs")
	viper.AddConfigPath("/etc/kubepilot")

	// Set defaults
	setDefaults()

	// Enable environment variables
	viper.AutomaticEnv()
	viper.SetEnvPrefix("KUBEPILOT")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// 验证配置
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// EncryptKey returns the key used to seal kubeconfig (and similar secrets) at rest.
// Prefer security.encrypt_key; fall back to jwt.secret for older deployments that
// historically reused the JWT secret so existing ciphertext keeps decrypting.
func (c *Config) EncryptKey() string {
	if key := strings.TrimSpace(c.Security.EncryptKey); key != "" {
		return key
	}
	return strings.TrimSpace(c.JWT.Secret)
}

// ModuleEnabled reports whether an internal module should run.
// Missing or unset entries default to enabled for backward compatibility.
func (c *Config) ModuleEnabled(name string) bool {
	if c == nil || c.Modules == nil {
		return true
	}
	mc, ok := c.Modules[name]
	if !ok || mc.Enabled == nil {
		return true
	}
	return *mc.Enabled
}

// ModuleSettings returns per-module config (zero value when unset).
func (c *Config) ModuleSettings(name string) ModuleConfig {
	if c == nil || c.Modules == nil {
		return ModuleConfig{}
	}
	return c.Modules[name]
}

// Validate 验证配置
func (c *Config) Validate() error {
	secret := strings.TrimSpace(c.JWT.Secret)
	if secret == "" || secret == "kubepilot-secret-key" {
		return fmt.Errorf("jwt.secret must be set to a non-default value for security")
	}
	if len(secret) < 16 {
		return fmt.Errorf("jwt.secret must be at least 16 characters")
	}

	encryptKey := strings.TrimSpace(c.Security.EncryptKey)
	if encryptKey != "" {
		if encryptKey == "kubepilot-encrypt-key" || encryptKey == "change-me" {
			return fmt.Errorf("security.encrypt_key must be set to a non-default value for security")
		}
		if len(encryptKey) < 16 {
			return fmt.Errorf("security.encrypt_key must be at least 16 characters")
		}
	}

	// 检查数据库配置
	if c.Database.Driver != "" && c.Database.Driver != "postgres" {
		return fmt.Errorf("database.driver %q is not supported; only postgres is available", c.Database.Driver)
	}
	if c.Database.Host == "" {
		return fmt.Errorf("database.host is required")
	}

	return nil
}

func setDefaults() {
	// Server defaults
	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.mode", "debug")
	// AI Agent / LLM 调用可能超过 30s；WriteTimeout 覆盖整个 handler 生命周期
	viper.SetDefault("server.read_timeout", 60*time.Second)
	viper.SetDefault("server.write_timeout", 300*time.Second)

	// Database defaults
	viper.SetDefault("database.driver", "postgres")
	viper.SetDefault("database.host", "localhost")
	viper.SetDefault("database.port", 5432)
	viper.SetDefault("database.username", "kubepilot")
	viper.SetDefault("database.password", "kubepilot")
	viper.SetDefault("database.dbname", "kubepilot")
	viper.SetDefault("database.sslmode", "disable")
	viper.SetDefault("database.max_idle_conns", 10)
	viper.SetDefault("database.max_open_conns", 100)

	// Redis defaults
	viper.SetDefault("redis.host", "localhost")
	viper.SetDefault("redis.port", 6379)
	viper.SetDefault("redis.password", "")
	viper.SetDefault("redis.db", 0)

	// JWT defaults
	viper.SetDefault("jwt.secret", "kubepilot-secret-key")
	viper.SetDefault("jwt.expire_time", 24*time.Hour)
	viper.SetDefault("jwt.issuer", "kubepilot")

	// Security defaults (empty = fall back to jwt.secret for compatibility)
	viper.SetDefault("security.encrypt_key", "")

	// Log defaults
	viper.SetDefault("log.level", "info")
	viper.SetDefault("log.format", "json")
	viper.SetDefault("log.output", "stdout")

	// K8S defaults
	viper.SetDefault("k8s.default_namespace", "default")
	viper.SetDefault("k8s.qps", 50.0)
	viper.SetDefault("k8s.burst", 100)

	// Cache defaults
	viper.SetDefault("cache.type", "memory")
	viper.SetDefault("cache.addr", "localhost:6379")
	viper.SetDefault("cache.password", "")
	viper.SetDefault("cache.db", 0)
}

func (d *DatabaseConfig) DSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.Username, d.Password, d.DBName, d.SSLMode)
}

func (r *RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%d", r.Host, r.Port)
}
