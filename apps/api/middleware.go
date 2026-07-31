package main

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

// HTTP middleware and response plumbing shared by every route.

type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (w *responseRecorder) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
		w.ResponseWriter.WriteHeader(status)
	}
}

func (w *responseRecorder) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func requestMiddleware(logger *slog.Logger, limiter *boundedRateLimiter, next http.Handler) http.Handler {
	authLimiter := newBoundedRateLimiter(apiRateLimitMaxEntries, apiAuthLimit, apiRateLimitWindow)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if !validRequestID(requestID) {
			requestID = newUUID()
		}
		w.Header().Set("X-Request-ID", requestID)

		if class, limited := rateLimitClass(r); limited {
			limit := limiter
			if class == "auth" {
				limit = authLimiter
			}
			if !limit.allow(class + ":" + rateLimitClient(r)) {
				metrics.rateLimited.Add(1)
				w.Header().Set("Retry-After", strconv.Itoa(int(apiRateLimitWindow/time.Second)))
				writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded", "request_id": requestID})
				return
			}
		}

		started := time.Now()
		rw := &responseRecorder{ResponseWriter: w}
		next.ServeHTTP(rw, r)
		if rw.status == 0 {
			rw.status = http.StatusOK
		}
		logger.Info("request", "request_id", requestID, "method", r.Method, "path", r.URL.Path, "status", rw.status, "duration_ms", time.Since(started).Milliseconds())
		metrics.observe(rw.status, time.Since(started))
	})
}

func validRequestID(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if r < 0x21 || r > 0x7e {
			return false
		}
	}
	return true
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		origin := r.Header.Get("Origin")
		if origin != "" && origin == envOrDefault("VPS_WEB_ORIGIN", "http://localhost:3000") {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-VPS-User, X-VPS-Runner-Token, X-VPS-Internal-Secret, X-VPS-OIDC-Subject, X-VPS-OIDC-Email")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
