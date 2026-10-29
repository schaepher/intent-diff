package gateway

// tokenBucket 是令牌桶限流器（基础实现，由后续意图启用）。
type tokenBucket struct{ tokens int }

func newTokenBucket(c Config) *tokenBucket { return &tokenBucket{tokens: c.TimeoutSeconds} }

// Allow 判断是否还有可用令牌。
func (b *tokenBucket) Allow() bool { return b.tokens > 0 }
