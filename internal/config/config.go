package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Redis    RedisConfig    `mapstructure:"redis"`
	JWT      JWTConfig      `mapstructure:"jwt"`
	Log      LogConfig      `mapstructure:"log"`
	K8S      K8SConfig      `mapstructure:"k8s"`
	LLM      LLMConfig      `mapstructure:"llm"`
	Cache    CacheConfig    `mapstructure:"cache"`
}

type ServerConfig struct {
	Host         string        `mapstructure:"host"`
	Port         int           `mapstructure:"port"`
	Mode         string        `mapstructure:"mode"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
}

type DatabaseConfig struct {
	Driver       string `mapstructure:"driver"` // postgres, mysql, sqlite
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

// Validate 验证配置
func (c *Config) Validate() error {
	// 检查 JWT 密钥是否为默认值
	defaultSecrets := []string{
		"kubepilot-secret-key",
		"kubepilot-secret-key-change-me",
		"",
	}
	for _, s := range defaultSecrets {
		if c.JWT.Secret == s {
			return fmt.Errorf("jwt.secret must be set to a non-default value for security")
		}
	}

	// 检查数据库配置
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
	viper.SetDefault("server.read_timeout", 30*time.Second)
	viper.SetDefault("server.write_timeout", 30*time.Second)

	// Database defaults
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
	switch d.Driver {
	case "mysql":
		// MySQL DSN: user:password@tcp(host:port)/dbname?charset=utf8mb4&parseTime=True&loc=Local
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			d.Username, d.Password, d.Host, d.Port, d.DBName)
	case "sqlite":
		// SQLite uses DBName as file path
		if d.DBName == "" {
			return "kubepilot.db"
		}
		return d.DBName
	default: // postgres
		return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
			d.Host, d.Port, d.Username, d.Password, d.DBName, d.SSLMode)
	}
}

func (r *RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%d", r.Host, r.Port)
}
