// Package cors lets browser pages of allowed origins call the API's REST
// routes: it answers their preflights and marks the responses they may read.
// Callers authenticate with bearer tokens, never cookies, so credentials are
// never allowed.
package cors

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/diegobermudez03/playhoot/api/internal/routing"
)

// AnyOrigin, as the only entry of a list of origins, allows every origin.
const AnyOrigin = "*"

// maxAge is how long a browser may reuse a preflight's answer.
const maxAge = time.Hour

const (
	allowedHeaders = "Authorization, Content-Type"
	// exposedHeaders are the response headers beyond the CORS-safelisted
	// ones a page may read.
	exposedHeaders = "Retry-After"
)

// ParseOrigins parses a comma-separated list of origins (scheme://host with
// an optional port, http or https) into their canonical form, or "*" alone
// for any origin. Empty entries are ignored; an empty list allows none.
func ParseOrigins(raw string) ([]string, error) {
	var origins []string
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if entry == AnyOrigin {
			origins = append(origins, AnyOrigin)
			continue
		}
		origin, err := canonicalOrigin(entry)
		if err != nil {
			return nil, err
		}
		origins = append(origins, origin)
	}
	for _, origin := range origins {
		if origin == AnyOrigin && len(origins) > 1 {
			return nil, errors.New(`"*" allows any origin and cannot be listed with others`)
		}
	}
	return origins, nil
}

func canonicalOrigin(entry string) (string, error) {
	parsed, err := url.Parse(entry)
	invalid := err != nil ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" || parsed.User != nil || parsed.Path != "" ||
		parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" ||
		strings.ContainsAny(parsed.Host, "*?\\")
	if invalid {
		return "", fmt.Errorf("%q is not an origin such as https://play.example.com", entry)
	}
	return parsed.Scheme + "://" + strings.ToLower(parsed.Host), nil
}

// Policy is which origins may call the REST routes cross-origin.
type Policy struct {
	anyOrigin bool
	origins   map[string]bool
}

// NewPolicy allows origins, as ParseOrigins returns them.
func NewPolicy(origins []string) Policy {
	policy := Policy{origins: make(map[string]bool, len(origins))}
	for _, origin := range origins {
		if origin == AnyOrigin {
			policy.anyOrigin = true
		}
		policy.origins[origin] = true
	}
	return policy
}

// allows reports whether a request from origin may be read cross-origin.
// Browsers send origins already in canonical form.
func (p Policy) allows(origin string) bool {
	return origin != "" && (p.anyOrigin || p.origins[strings.ToLower(origin)])
}

// Responses lets a page of an allowed origin read every response of the
// route, refusals included; requests are served the same whatever their
// origin, since only browsers enforce CORS.
func (p Policy) Responses() routing.Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			header := w.Header()
			header.Add("Vary", "Origin")
			if origin := r.Header.Get("Origin"); p.allows(origin) {
				header.Set("Access-Control-Allow-Origin", origin)
				header.Set("Access-Control-Expose-Headers", exposedHeaders)
			}
			next(w, r)
		}
	}
}

// Preflight answers a browser asking whether a page of its origin may call
// the path with one of methods; a disallowed origin gets no permission.
func (p Policy) Preflight(methods []string) http.HandlerFunc {
	allowedMethods := strings.Join(methods, ", ")
	return func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Add("Vary", "Origin")
		header.Add("Vary", "Access-Control-Request-Method")
		header.Add("Vary", "Access-Control-Request-Headers")
		if origin := r.Header.Get("Origin"); p.allows(origin) {
			header.Set("Access-Control-Allow-Origin", origin)
			header.Set("Access-Control-Allow-Methods", allowedMethods)
			header.Set("Access-Control-Allow-Headers", allowedHeaders)
			header.Set("Access-Control-Max-Age", strconv.Itoa(int(maxAge.Seconds())))
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
