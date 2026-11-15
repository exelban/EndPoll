package check

import "net/http"

// Status codes used for check failures that have no HTTP response.
const (
	StatusWebServerDown      = 521
	StatusConnectionTimeout  = 522
	StatusOriginUnreachable  = 523
	StatusTLSHandshakeFailed = 525
)

// StatusText - returns the description of a check or HTTP status code.
func StatusText(code int) string {
	switch code {
	case StatusWebServerDown:
		return "Web server is down"
	case StatusConnectionTimeout:
		return "Connection timed out"
	case StatusOriginUnreachable:
		return "Origin is unreachable"
	default:
		return http.StatusText(code)
	}
}
