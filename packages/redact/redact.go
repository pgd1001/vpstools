package redact

import (
	"regexp"
	"strings"
)

var patterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(api[_-]?key|apikey|api_secret|secret[_-]?key|token|password|passwd|authorization|auth)[=:]\s*["'\x60]?([^\s"'\x60}]+)["'\x60]?`),
	regexp.MustCompile(`(?i)(Bearer\s+)([\w-]+\.[\w-]+\.[\w-]+)`),
	regexp.MustCompile(`(?i)(-----BEGIN\s+(?:RSA\s+)?PRIVATE\s+KEY-----)([\s\S]*?)(-----END\s+(?:RSA\s+)?PRIVATE\s+KEY-----)`),
	regexp.MustCompile(`(?i)(ssh-rsa\s+AAAA[^\s]+)`),
	regexp.MustCompile(`(?i)(DATABASE_URL|DB_URL|PGPASSWORD|MYSQL_PWD|REDIS_URL)\s*=\s*["'\x60]?([^\s"'\x60]+)["'\x60]?`),
	regexp.MustCompile(`eyJ[A-Za-z0-9-_]+\.eyJ[A-Za-z0-9-_]+\.[A-Za-z0-9-_]+`),
	regexp.MustCompile(`(?i)(authorization|proxy-authorization):\s*["'\x60]?[^\s"'\x60]+["'\x60]?`),
	regexp.MustCompile(`\b(ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9_]{36,}\b`),
	regexp.MustCompile(`\b(xox[baprs]-[A-Za-z0-9-]+)\b`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
}

func Stdout(s string) string {
	out := s
	for _, p := range patterns {
		out = p.ReplaceAllStringFunc(out, func(match string) string {
			parts := p.FindStringSubmatch(match)
			if len(parts) >= 3 && parts[0] == match && strings.Count(match, parts[2]) == 1 {
				return strings.Replace(match, parts[2], "***REDACTED***", 1)
			}
			if len(parts) >= 2 && strings.Count(match, parts[1]) == 1 {
				return strings.Replace(match, parts[1], strings.Repeat("*", min(len(parts[1]), 20)), 1)
			}
			if len(match) > 20 {
				return match[:8] + "***REDACTED***"
			}
			return "***REDACTED***"
		})
	}
	return out
}
