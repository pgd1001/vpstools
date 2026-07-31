package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Method routing is now the mux's responsibility rather than a switch inside
// every handler. These tests pin that behaviour so a route declared with the
// wrong method or pattern fails here instead of in production.
func TestUnsupportedMethodsAreRejectedPerRoute(t *testing.T) {
	_, mux, cleanup := testAPI(t)
	defer cleanup()

	cases := []struct{ method, path string }{
		{http.MethodDelete, "/api/v1/servers"},
		{http.MethodPost, "/api/v1/whoami"},
		{http.MethodGet, "/api/v1/auth/tokens"},
		{http.MethodPut, "/api/v1/executions"},
		{http.MethodGet, "/api/v1/executions/exe_x/cancel"},
		{http.MethodDelete, "/api/v1/audit"},
		{http.MethodPost, "/api/v1/approvals"},
		{http.MethodGet, "/api/v1/automation/pause"},
		{http.MethodPost, "/api/v1/schedules/sch_x"},
	}
	for _, testCase := range cases {
		request := httptest.NewRequest(testCase.method, testCase.path, nil)
		request.Header.Set("X-VPS-User", "user_senior")
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s = %d, want 405", testCase.method, testCase.path, recorder.Code)
		}
	}
}

// Path segments must be extracted by the mux, including for nested action
// routes, so a handler receives the identifier rather than a partially sliced
// path.
func TestPathSegmentsReachHandlersIntact(t *testing.T) {
	var captured string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/things/{thingID}", pathHandler("thingID", func(w http.ResponseWriter, r *http.Request, value string) {
		captured = value
		writeJSON(w, http.StatusOK, map[string]string{"id": value})
	}))
	mux.HandleFunc("POST /api/v1/things/{thingID}/act", pathHandler("thingID", func(w http.ResponseWriter, r *http.Request, value string) {
		captured = value
		writeJSON(w, http.StatusOK, map[string]string{"id": value})
	}))

	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/things/thing_123", nil))
	if captured != "thing_123" {
		t.Fatalf("plain segment = %q, want thing_123", captured)
	}

	// The old suffix-slicing routing derived the id by trimming "/act" from
	// the path. Pattern matching must produce the same id without that step.
	captured = ""
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/things/thing_456/act", nil))
	if captured != "thing_456" {
		t.Fatalf("action-route segment = %q, want thing_456", captured)
	}
}

// A nested action route must win over the bare identifier route, otherwise
// "/executions/{id}/cancel" would be read as an execution whose id contains a
// slash.
func TestActionRoutesTakePrecedenceOverIdentifierRoutes(t *testing.T) {
	matched := ""
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/things/{thingID}", func(w http.ResponseWriter, r *http.Request) { matched = "identifier" })
	mux.HandleFunc("POST /api/v1/things/{thingID}/act", func(w http.ResponseWriter, r *http.Request) { matched = "action" })

	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/things/thing_1/act", nil))
	if matched != "action" {
		t.Fatalf("matched %q route, want the action route", matched)
	}
}

// An empty path segment must not reach a handler as a valid identifier.
func TestEmptyPathSegmentIsRejected(t *testing.T) {
	reached := false
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/things/{thingID}", pathHandler("thingID", func(w http.ResponseWriter, r *http.Request, value string) {
		reached = true
	}))

	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/things/", nil))
	if reached {
		t.Fatal("a handler received an empty identifier")
	}
	if recorder.Code == http.StatusOK {
		t.Fatalf("empty identifier returned %d, want a client error", recorder.Code)
	}
}

// Every route the CLI, web console, and runner depend on must be registered.
// This is the check that the split did not silently drop an endpoint.
func TestAllDocumentedRoutesAreRegistered(t *testing.T) {
	_, mux, cleanup := testAPI(t)
	defer cleanup()

	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/health"},
		{http.MethodGet, "/api/v1/ready"},
		{http.MethodGet, "/metrics"},
		{http.MethodGet, "/api/v1/whoami"},
		{http.MethodPost, "/api/v1/ai/analyze"},
		{http.MethodPost, "/api/v1/auth/tokens"},
		{http.MethodDelete, "/api/v1/auth/tokens/pat_x"},
		{http.MethodGet, "/api/v1/servers"},
		{http.MethodPost, "/api/v1/servers"},
		{http.MethodGet, "/api/v1/servers/srv_demo"},
		{http.MethodPatch, "/api/v1/servers/srv_demo"},
		{http.MethodPut, "/api/v1/servers/srv_demo"},
		{http.MethodDelete, "/api/v1/servers/srv_demo"},
		{http.MethodPost, "/api/v1/servers/srv_demo/check"},
		{http.MethodGet, "/api/v1/runners"},
		{http.MethodPost, "/api/v1/runners"},
		{http.MethodPost, "/api/v1/runners/heartbeat"},
		{http.MethodPost, "/api/v1/runners/registration-token"},
		{http.MethodPost, "/api/v1/runners/manage"},
		{http.MethodPatch, "/api/v1/runners/rnr_local"},
		{http.MethodDelete, "/api/v1/runners/rnr_local"},
		{http.MethodPost, "/api/v1/runners/rnr_local/rotate-token"},
		{http.MethodGet, "/api/v1/executions"},
		{http.MethodPost, "/api/v1/executions"},
		{http.MethodGet, "/api/v1/executions/exe_x"},
		{http.MethodPost, "/api/v1/executions/exe_x/cancel"},
		{http.MethodGet, "/api/v1/jobs/next"},
		{http.MethodPost, "/api/v1/jobs/result"},
		{http.MethodPost, "/api/v1/jobs/renew"},
		{http.MethodGet, "/api/v1/audit"},
		{http.MethodGet, "/api/v1/audit/verify"},
		{http.MethodGet, "/api/v1/runbooks"},
		{http.MethodPost, "/api/v1/runbooks"},
		{http.MethodGet, "/api/v1/runbooks/check-uptime"},
		{http.MethodPut, "/api/v1/runbooks/check-uptime"},
		{http.MethodDelete, "/api/v1/runbooks/check-uptime"},
		{http.MethodPost, "/api/v1/runbooks/check-uptime/run"},
		{http.MethodPost, "/api/v1/runbooks/check-uptime/publish"},
		{http.MethodGet, "/api/v1/approvals"},
		{http.MethodGet, "/api/v1/approvals/apr_x"},
		{http.MethodPost, "/api/v1/approvals/apr_x/approve"},
		{http.MethodPost, "/api/v1/approvals/apr_x/deny"},
		{http.MethodGet, "/api/v1/schedules"},
		{http.MethodPost, "/api/v1/schedules"},
		{http.MethodDelete, "/api/v1/schedules/sch_x"},
		{http.MethodGet, "/api/v1/automation/status"},
		{http.MethodPost, "/api/v1/automation/pause"},
		{http.MethodPost, "/api/v1/automation/resume"},
	}

	for _, route := range routes {
		request := httptest.NewRequest(route.method, route.path, nil)
		_, pattern := mux.Handler(request)
		if pattern == "" {
			t.Errorf("no route registered for %s %s", route.method, route.path)
		}
	}
}
