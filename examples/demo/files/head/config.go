package gateway

// Config 是网关配置。
type Config struct {
	TimeoutSeconds  int
	RateLimitPerSec int
}

var cfg = Config{TimeoutSeconds: 3, RateLimitPerSec: 50}
