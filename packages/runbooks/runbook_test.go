package runbooks

import (
	"strings"
	"testing"
)

func TestRenderCommandValidatesAndQuotesParameters(t *testing.T) {
	rb := &Runbook{
		APIVersion: "vps-tools.io/v1",
		Kind:       "Runbook",
		Metadata:   Metadata{Name: "restart-service", Title: "Restart service", Risk: RiskLow},
		Spec: Spec{
			Parameters: []Parameter{{Name: "service", Type: "enum", Required: true, AllowedValues: []string{"nginx", "api"}}},
			Execution:  Execution{Command: "systemctl restart ${service}"},
		},
	}
	if err := Validate(rb); err != nil {
		t.Fatalf("validate runbook: %v", err)
	}
	command, err := rb.RenderCommand(map[string]string{"service": "nginx"})
	if err != nil {
		t.Fatalf("render command: %v", err)
	}
	if command != "systemctl restart 'nginx'" {
		t.Fatalf("unexpected command: %q", command)
	}

	if _, err := rb.RenderCommand(map[string]string{"service": "nginx; rm -rf /"}); err == nil {
		t.Fatal("expected disallowed enum value to fail")
	}
}

func TestRenderCommandQuotesShellCharacters(t *testing.T) {
	rb := &Runbook{
		APIVersion: "vps-tools.io/v1",
		Kind:       "Runbook",
		Metadata:   Metadata{Name: "echo-value", Title: "Echo value", Risk: RiskLow},
		Spec: Spec{
			Parameters: []Parameter{{Name: "value", Type: "string", Required: true}},
			Execution:  Execution{Command: "printf '%s' ${value}"},
		},
	}
	command, err := rb.RenderCommand(map[string]string{"value": "$(touch /tmp/pwned)"})
	if err != nil {
		t.Fatalf("render command: %v", err)
	}
	if command != "printf '%s' '$(touch /tmp/pwned)'" {
		t.Fatalf("parameter was not safely quoted: %q", command)
	}
}

func TestRenderCommandUsesDefaultsAndRejectsMissingValues(t *testing.T) {
	rb := &Runbook{
		APIVersion: "vps-tools.io/v1",
		Kind:       "Runbook",
		Metadata:   Metadata{Name: "health-check", Title: "Health check", Risk: RiskLow},
		Spec: Spec{
			Parameters: []Parameter{{Name: "path", Type: "string", Default: "/health"}, {Name: "token", Type: "string", Required: true}},
			Execution:  Execution{Command: "curl -H ${token} ${path}"},
		},
	}
	if _, err := rb.RenderCommand(nil); err == nil {
		t.Fatal("expected missing required parameter to fail")
	}
	command, err := rb.RenderCommand(map[string]string{"token": "Authorization: Bearer test"})
	if err != nil {
		t.Fatalf("render command with required value: %v", err)
	}
	if !strings.Contains(command, "'/health'") || !strings.Contains(command, "'Authorization: Bearer test'") {
		t.Fatalf("defaults or supplied value not rendered safely: %q", command)
	}
}

func TestValidateRejectsInvalidParameterDefinitions(t *testing.T) {
	rb := &Runbook{
		APIVersion: "vps-tools.io/v1",
		Kind:       "Runbook",
		Metadata:   Metadata{Name: "bad-params", Title: "Bad params", Risk: RiskLow},
		Spec: Spec{
			Parameters: []Parameter{{Name: "bad-name", Type: "integer"}},
			Execution:  Execution{Command: "echo ok"},
		},
	}
	if err := Validate(rb); err == nil {
		t.Fatal("expected invalid parameter name to fail")
	}
}

func TestParseParameterValuesRejectsMalformedAndDuplicateEntries(t *testing.T) {
	if _, err := ParseParameterValues("service"); err == nil {
		t.Fatal("expected malformed parameter to fail")
	}
	if _, err := ParseParameterValues("service=api,service=web"); err == nil {
		t.Fatal("expected duplicate parameter to fail")
	}
}

func TestParseParameterValuesKeepsEmptyValues(t *testing.T) {
	params, err := ParseParameterValues("note=")
	if err != nil {
		t.Fatalf("parse parameters: %v", err)
	}
	if value, ok := params["note"]; !ok || value != "" {
		t.Fatalf("expected empty value to be retained, got %#v", params)
	}
}
