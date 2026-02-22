package middleware

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/auth"
	"github.com/openpaw/openpaw/internal/logger"
)

type contextKey string

const (
	UserIDKey    contextKey = "userID"
	UsernameKey  contextKey = "username"
	RequestIDKey contextKey = "requestID"
)

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := uuid.New().String()
		ctx := context.WithValue(r.Context(), RequestIDKey, id)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(wrapped, r)
		logger.HTTP(r.Method, r.URL.Path, wrapped.statusCode, time.Since(start))
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Hijack implements http.Hijacker, required for WebSocket upgrades.
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not implement http.Hijacker")
}

// Flush implements http.Flusher for streaming responses.
func (rw *responseWriter) Flush() {
	if fl, ok := rw.ResponseWriter.(http.Flusher); ok {
		fl.Flush()
	}
}

func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:5173")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-CSRF-Token")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func Auth(authService *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := ""

			// Check Authorization header first
			if authHeader := r.Header.Get("Authorization"); authHeader != "" {
				parts := strings.SplitN(authHeader, " ", 2)
				if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
					tokenStr = parts[1]
				}
			}

			// Fall back to cookie
			if tokenStr == "" {
				if cookie, err := r.Cookie("openpaw_token"); err == nil {
					tokenStr = cookie.Value
				}
			}

			if tokenStr == "" {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			claims, err := authService.ValidateToken(tokenStr)
			if err != nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
			ctx = context.WithValue(ctx, UsernameKey, claims.Username)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetUserID(ctx context.Context) string {
	if v, ok := ctx.Value(UserIDKey).(string); ok {
		return v
	}
	return ""
}

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; style-src-elem 'self' 'unsafe-inline' https://fonts.googleapis.com; img-src 'self' data: blob:; connect-src 'self' ws: wss:; font-src 'self' data: https://fonts.gstatic.com; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

type rateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	limit    int
	window   time.Duration
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	rl := &rateLimiter{
		attempts: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
	go rl.cleanup()
	return rl
}

func (rl *rateLimiter) cleanup() {
	for {
		time.Sleep(rl.window)
		rl.mu.Lock()
		now := time.Now()
		for ip, times := range rl.attempts {
			var valid []time.Time
			for _, t := range times {
				if now.Sub(t) < rl.window {
					valid = append(valid, t)
				}
			}
			if len(valid) == 0 {
				delete(rl.attempts, ip)
			} else {
				rl.attempts[ip] = valid
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	var valid []time.Time
	for _, t := range rl.attempts[ip] {
		if now.Sub(t) < rl.window {
			valid = append(valid, t)
		}
	}
	if len(valid) >= rl.limit {
		rl.attempts[ip] = valid
		return false
	}
	rl.attempts[ip] = append(valid, now)
	return true
}

func RateLimit(limit int, window time.Duration) func(http.Handler) http.Handler {
	rl := newRateLimiter(limit, window)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := r.RemoteAddr
			if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
				ip = strings.Split(fwd, ",")[0]
			}
			ip = strings.TrimSpace(ip)
			if host, _, err := net.SplitHostPort(ip); err == nil {
				ip = host
			}
			if !rl.allow(ip) {
				http.Error(w, `{"error":"too many requests"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func CSRFProtection(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" || r.Method == "HEAD" || r.Method == "OPTIONS" {
			next.ServeHTTP(w, r)
			return
		}

		cookie, err := r.Cookie("openpaw_csrf")
		if err != nil || cookie.Value == "" {
			http.Error(w, `{"error":"missing CSRF token"}`, http.StatusForbidden)
			return
		}

		header := r.Header.Get("X-CSRF-Token")
		if header == "" || header != cookie.Value {
			http.Error(w, `{"error":"invalid CSRF token"}`, http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func SetCSRFCookie(w http.ResponseWriter, r *http.Request) string {
	token := uuid.New().String()
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     "openpaw_csrf",
		Value:    token,
		Path:     "/",
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		MaxAge:   86400 * 30,
	})
	return token
}

