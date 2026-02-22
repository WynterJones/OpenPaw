package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/openpaw/openpaw/internal/auth"
)

// okHandler is a simple handler that writes 200 OK for middleware chain tests.
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

// --- RequestID ---

func TestRequestID_AddsHeader(t *testing.T) {
	handler := RequestID(okHandler)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	id := rr.Header().Get("X-Request-ID")
	if id == "" {
		t.Fatal("expected X-Request-ID header to be set, got empty string")
	}
	// UUID v4 is 36 characters (8-4-4-4-12)
	if len(id) != 36 {
		t.Fatalf("expected UUID-length X-Request-ID, got %q (len %d)", id, len(id))
	}
}

func TestRequestID_UniquePerRequest(t *testing.T) {
	handler := RequestID(okHandler)

	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	if rr1.Header().Get("X-Request-ID") == rr2.Header().Get("X-Request-ID") {
		t.Fatal("expected unique X-Request-ID per request")
	}
}

// --- CORS ---

func TestCORS_DevOrigin(t *testing.T) {
	handler := CORS(okHandler)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	expected := map[string]string{
		"Access-Control-Allow-Origin":      "http://localhost:5173",
		"Access-Control-Allow-Methods":     "GET, POST, PUT, DELETE, OPTIONS",
		"Access-Control-Allow-Headers":     "Content-Type, Authorization, X-CSRF-Token",
		"Access-Control-Allow-Credentials": "true",
		"Access-Control-Max-Age":           "86400",
	}

	for header, want := range expected {
		got := rr.Header().Get(header)
		if got != want {
			t.Errorf("header %s = %q, want %q", header, got, want)
		}
	}

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}

func TestCORS_NullOrigin(t *testing.T) {
	handler := CORS(okHandler)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboards/123/assets/index.html", nil)
	req.Header.Set("Origin", "null")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "null" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "null")
	}
	if got := rr.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("Access-Control-Allow-Credentials should not be set for null origin, got %q", got)
	}
}

func TestCORS_SameOriginNoHeader(t *testing.T) {
	handler := CORS(okHandler)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin should not be set for same-origin, got %q", got)
	}
}

func TestCORS_PreflightOptions(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("inner handler should not be called on OPTIONS preflight")
	})

	handler := CORS(inner)
	req := httptest.NewRequest(http.MethodOptions, "/api/test", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected status 204 for OPTIONS, got %d", rr.Code)
	}
}

// --- Auth ---

func newTestAuthService() *auth.Service {
	return auth.NewService("test-secret-key-for-unit-tests")
}

func TestAuth_ValidTokenInCookie(t *testing.T) {
	svc := newTestAuthService()
	token, err := svc.GenerateToken("user-123", "alice")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := GetUserID(r.Context())
		if userID != "user-123" {
			t.Errorf("expected userID %q, got %q", "user-123", userID)
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := Auth(svc)(inner)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "openpaw_token", Value: token})
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}

func TestAuth_ValidTokenInBearerHeader(t *testing.T) {
	svc := newTestAuthService()
	token, err := svc.GenerateToken("user-456", "bob")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := GetUserID(r.Context())
		if userID != "user-456" {
			t.Errorf("expected userID %q, got %q", "user-456", userID)
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := Auth(svc)(inner)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}

func TestAuth_MissingToken_Returns401(t *testing.T) {
	svc := newTestAuthService()
	handler := Auth(svc)(okHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rr.Code)
	}
}

func TestAuth_InvalidToken_Returns401(t *testing.T) {
	svc := newTestAuthService()
	handler := Auth(svc)(okHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer totally-not-a-valid-jwt")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rr.Code)
	}
}

func TestAuth_ExpiredToken_Returns401(t *testing.T) {
	svc := newTestAuthService()
	// Generate a token that expired 1 hour ago.
	token, err := svc.GenerateTokenWithTTL("user-expired", "expired", -1*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	handler := Auth(svc)(okHandler)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rr.Code)
	}
}

func TestAuth_BearerHeaderTakesPrecedenceOverCookie(t *testing.T) {
	svc := newTestAuthService()

	headerToken, _ := svc.GenerateToken("header-user", "header")
	cookieToken, _ := svc.GenerateToken("cookie-user", "cookie")

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := GetUserID(r.Context())
		if userID != "header-user" {
			t.Errorf("expected header-user to take precedence, got %q", userID)
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := Auth(svc)(inner)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+headerToken)
	req.AddCookie(&http.Cookie{Name: "openpaw_token", Value: cookieToken})
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}

// --- SecurityHeaders ---

func TestSecurityHeaders_SetsAllHeaders(t *testing.T) {
	handler := SecurityHeaders(okHandler)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	headers := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":       "DENY",
		"Referrer-Policy":       "strict-origin-when-cross-origin",
		"Permissions-Policy":    "camera=(), microphone=(), geolocation=()",
	}

	for header, want := range headers {
		got := rr.Header().Get(header)
		if got != want {
			t.Errorf("header %s = %q, want %q", header, got, want)
		}
	}

	csp := rr.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Error("expected Content-Security-Policy header to be set")
	}
	// Verify it contains key directives.
	for _, directive := range []string{"default-src", "script-src", "frame-ancestors 'none'"} {
		if !contains(csp, directive) {
			t.Errorf("CSP missing directive %q", directive)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// --- RateLimit ---

func TestRateLimit_AllowsRequestsUnderLimit(t *testing.T) {
	limiter := RateLimit(5, time.Minute)
	handler := limiter(okHandler)

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("request %d: expected status 200, got %d", i+1, rr.Code)
		}
	}
}

func TestRateLimit_BlocksRequestsOverLimit(t *testing.T) {
	limiter := RateLimit(3, time.Minute)
	handler := limiter(okHandler)

	// Use up all 3 allowed requests.
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "10.0.0.1:9999"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("request %d should be allowed, got %d", i+1, rr.Code)
		}
	}

	// The 4th request should be rejected.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:9999"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("expected status 429, got %d", rr.Code)
	}
}

func TestRateLimit_DifferentIPsTrackedSeparately(t *testing.T) {
	limiter := RateLimit(1, time.Minute)
	handler := limiter(okHandler)

	// First IP uses its single allowed request.
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.RemoteAddr = "1.1.1.1:1111"
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("first IP first request should be allowed, got %d", rr1.Code)
	}

	// Second IP should still be allowed.
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "2.2.2.2:2222"
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Errorf("second IP should be allowed, got %d", rr2.Code)
	}
}

func TestRateLimit_UsesXForwardedFor(t *testing.T) {
	limiter := RateLimit(1, time.Minute)
	handler := limiter(okHandler)

	// First request with X-Forwarded-For uses the limit.
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.RemoteAddr = "127.0.0.1:8080"
	req1.Header.Set("X-Forwarded-For", "203.0.113.50")
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("first request should be allowed, got %d", rr1.Code)
	}

	// Second request from same forwarded IP should be blocked.
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "127.0.0.1:8080"
	req2.Header.Set("X-Forwarded-For", "203.0.113.50")
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 for repeated forwarded IP, got %d", rr2.Code)
	}
}

// --- CSRFProtection ---

func TestCSRFProtection_SkipsGETRequests(t *testing.T) {
	handler := CSRFProtection(okHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GET should pass without CSRF, got %d", rr.Code)
	}
}

func TestCSRFProtection_SkipsHEADRequests(t *testing.T) {
	handler := CSRFProtection(okHandler)

	req := httptest.NewRequest(http.MethodHead, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("HEAD should pass without CSRF, got %d", rr.Code)
	}
}

func TestCSRFProtection_SkipsOPTIONSRequests(t *testing.T) {
	handler := CSRFProtection(okHandler)

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("OPTIONS should pass without CSRF, got %d", rr.Code)
	}
}

func TestCSRFProtection_RejectsPOSTWithoutToken(t *testing.T) {
	handler := CSRFProtection(okHandler)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("POST without CSRF token should return 403, got %d", rr.Code)
	}
}

func TestCSRFProtection_RejectsPOSTWithCookieButNoHeader(t *testing.T) {
	handler := CSRFProtection(okHandler)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.AddCookie(&http.Cookie{Name: "openpaw_csrf", Value: "csrf-token-value"})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("POST with cookie but no header should return 403, got %d", rr.Code)
	}
}

func TestCSRFProtection_RejectsPOSTWithMismatchedTokens(t *testing.T) {
	handler := CSRFProtection(okHandler)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.AddCookie(&http.Cookie{Name: "openpaw_csrf", Value: "token-a"})
	req.Header.Set("X-CSRF-Token", "token-b")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("POST with mismatched tokens should return 403, got %d", rr.Code)
	}
}

func TestCSRFProtection_AllowsPOSTWithMatchingTokens(t *testing.T) {
	handler := CSRFProtection(okHandler)

	token := "valid-csrf-token-12345"
	req := httptest.NewRequest(http.MethodPost, "/api/action", nil)
	req.AddCookie(&http.Cookie{Name: "openpaw_csrf", Value: token})
	req.Header.Set("X-CSRF-Token", token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("POST with matching CSRF tokens should return 200, got %d", rr.Code)
	}
}

func TestCSRFProtection_RejectsDELETEWithoutToken(t *testing.T) {
	handler := CSRFProtection(okHandler)

	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("DELETE without CSRF token should return 403, got %d", rr.Code)
	}
}

func TestCSRFProtection_AllowsPUTWithMatchingTokens(t *testing.T) {
	handler := CSRFProtection(okHandler)

	token := "put-csrf-token"
	req := httptest.NewRequest(http.MethodPut, "/api/resource", nil)
	req.AddCookie(&http.Cookie{Name: "openpaw_csrf", Value: token})
	req.Header.Set("X-CSRF-Token", token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("PUT with matching CSRF tokens should return 200, got %d", rr.Code)
	}
}

// --- SetCSRFCookie ---

func TestSetCSRFCookie_SetsCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	token := SetCSRFCookie(rr, req)

	if token == "" {
		t.Fatal("expected non-empty CSRF token")
	}

	cookies := rr.Result().Cookies()
	var found *http.Cookie
	for _, c := range cookies {
		if c.Name == "openpaw_csrf" {
			found = c
			break
		}
	}

	if found == nil {
		t.Fatal("expected openpaw_csrf cookie to be set")
	}
	if found.Value != token {
		t.Errorf("cookie value %q does not match returned token %q", found.Value, token)
	}
	if found.HttpOnly {
		t.Error("CSRF cookie should not be HttpOnly (JS needs to read it)")
	}
	if found.Path != "/" {
		t.Errorf("expected cookie path %q, got %q", "/", found.Path)
	}
}

func TestSetCSRFCookie_NotSecureOverHTTP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	rr := httptest.NewRecorder()

	SetCSRFCookie(rr, req)

	cookies := rr.Result().Cookies()
	for _, c := range cookies {
		if c.Name == "openpaw_csrf" {
			if c.Secure {
				t.Error("CSRF cookie should not be Secure over plain HTTP")
			}
			return
		}
	}
	t.Fatal("openpaw_csrf cookie not found")
}

// --- GetUserID ---

func TestGetUserID_ExtractsFromContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), UserIDKey, "user-abc")
	got := GetUserID(ctx)
	if got != "user-abc" {
		t.Errorf("expected %q, got %q", "user-abc", got)
	}
}

func TestGetUserID_ReturnsEmptyWhenMissing(t *testing.T) {
	got := GetUserID(context.Background())
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestGetUserID_ReturnsEmptyForWrongType(t *testing.T) {
	ctx := context.WithValue(context.Background(), UserIDKey, 12345)
	got := GetUserID(ctx)
	if got != "" {
		t.Errorf("expected empty string for non-string value, got %q", got)
	}
}
