package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

// Rule is a fixed-window limit.
type Rule struct {
	Limit  int
	Window time.Duration
}

// pathRule binds a rule to a method and path prefix.
type pathRule struct {
	method string // empty matches any method
	prefix string
	rule   Rule
}

// rules are the limits from tech.md §9.6, most specific first.
var rules = []pathRule{
	{method: http.MethodPost, prefix: "/checkout", rule: Rule{Limit: 10, Window: time.Hour}},
	{method: http.MethodPost, prefix: "/admin/login", rule: Rule{Limit: 5, Window: 15 * time.Minute}},
	{prefix: "/webhooks/", rule: Rule{Limit: 120, Window: time.Minute}},
	{prefix: "/cart", rule: Rule{Limit: 60, Window: time.Minute}},
}

// Limiter counts requests per key in fixed windows. It is in-process on
// purpose: one app container, no shared state to keep consistent.
type Limiter struct {
	mu      sync.Mutex
	windows map[string]*window
	now     func() time.Time
}

type window struct {
	count int
	until time.Time
}

// NewLimiter builds an empty limiter.
func NewLimiter() *Limiter {
	return &Limiter{windows: make(map[string]*window), now: time.Now}
}

// Allow reports whether one more request fits into the rule for this key.
func (l *Limiter) Allow(key string, rule Rule) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	w, ok := l.windows[key]
	if !ok || now.After(w.until) {
		l.windows[key] = &window{count: 1, until: now.Add(rule.Window)}
		return true
	}
	if w.count >= rule.Limit {
		return false
	}
	w.count++
	return true
}

// Cleanup drops expired windows so the map does not grow without bound.
func (l *Limiter) Cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	for k, w := range l.windows {
		if now.After(w.until) {
			delete(l.windows, k)
		}
	}
}

// RateLimit applies the table above, keyed by client IP.
func RateLimit(l *Limiter) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rule, name, ok := match(r)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			if !l.Allow(name+"|"+ClientIP(r), rule) {
				w.Header().Set("Retry-After", "60")
				http.Error(w, "too many requests", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func match(r *http.Request) (Rule, string, bool) {
	for _, pr := range rules {
		if pr.method != "" && pr.method != r.Method {
			continue
		}
		if strings.HasPrefix(r.URL.Path, pr.prefix) {
			return pr.rule, pr.method + pr.prefix, true
		}
	}
	return Rule{}, "", false
}
