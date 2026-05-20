package runbooks

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type RiskLevel string

const (
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

type Runbook struct {
	APIVersion string   `yaml:"apiVersion" json:"apiVersion"`
	Kind       string   `yaml:"kind" json:"kind"`
	Metadata   Metadata `yaml:"metadata" json:"metadata"`
	Spec       Spec     `yaml:"spec" json:"spec"`
}

type Metadata struct {
	Name        string    `yaml:"name" json:"name"`
	Title       string    `yaml:"title" json:"title"`
	Description string    `yaml:"description" json:"description"`
	Risk        RiskLevel `yaml:"risk" json:"risk"`
	Tags        []string  `yaml:"tags" json:"tags"`
	Version     int       `yaml:"version" json:"version"`
}

type Spec struct {
	Parameters []Parameter  `yaml:"parameters" json:"parameters"`
	Targets    TargetRules  `yaml:"targets" json:"targets"`
	Approval   ApprovalRule `yaml:"approval" json:"approval"`
	Execution  Execution    `yaml:"execution" json:"execution"`
	Output     Output       `yaml:"output" json:"output"`
}

type Parameter struct {
	Name          string   `yaml:"name" json:"name"`
	Type          string   `yaml:"type" json:"type"`
	Default       string   `yaml:"default" json:"default"`
	AllowedValues []string `yaml:"allowedValues" json:"allowedValues"`
	Required      bool     `yaml:"required" json:"required"`
	Description   string   `yaml:"description" json:"description"`
}

type TargetRules struct {
	AllowedTags         map[string]string `yaml:"allowedTags" json:"allowedTags"`
	AllowedEnvironments []string          `yaml:"allowedEnvironments" json:"allowedEnvironments"`
	AllowedServers      []string          `yaml:"allowedServers" json:"allowedServers"`
}

type ApprovalRule struct {
	Required     bool   `yaml:"required" json:"required"`
	Environment  string `yaml:"environment" json:"environment"`
	RequiredRole string `yaml:"requiredRole" json:"requiredRole"`
}

type Execution struct {
	Shell          string `yaml:"shell" json:"shell"`
	Sudo           bool   `yaml:"sudo" json:"sudo"`
	TimeoutSeconds int    `yaml:"timeoutSeconds" json:"timeoutSeconds"`
	Concurrency    int    `yaml:"concurrency" json:"concurrency"`
	Command        string `yaml:"command" json:"command"`
}

type Output struct {
	Store  string   `yaml:"store" json:"store"`
	Redact []string `yaml:"redact" json:"redact"`
}

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("runbook validation error: %s: %s", e.Field, e.Message)
}

func Validate(rb *Runbook) error {
	if rb.APIVersion != "vps-tools.io/v1" {
		return &ValidationError{Field: "apiVersion", Message: "must be 'vps-tools.io/v1'"}
	}
	if rb.Kind != "Runbook" {
		return &ValidationError{Field: "kind", Message: "must be 'Runbook'"}
	}
	if rb.Metadata.Name == "" {
		return &ValidationError{Field: "metadata.name", Message: "is required"}
	}
	if !validName(rb.Metadata.Name) {
		return &ValidationError{Field: "metadata.name", Message: "must be lowercase alphanumeric with hyphens"}
	}
	if rb.Metadata.Title == "" {
		return &ValidationError{Field: "metadata.title", Message: "is required"}
	}
	if rb.Spec.Execution.Command == "" {
		return &ValidationError{Field: "spec.execution.command", Message: "is required"}
	}
	if rb.Spec.Execution.TimeoutSeconds < 0 {
		return &ValidationError{Field: "spec.execution.timeoutSeconds", Message: "must be >= 0"}
	}
	if rb.Spec.Execution.Concurrency < 0 {
		return &ValidationError{Field: "spec.execution.concurrency", Message: "must be >= 0"}
	}
	if rb.Metadata.Risk == "" {
		rb.Metadata.Risk = RiskMedium
	}
	return nil
}

func Parse(yamlContent string) (*Runbook, error) {
	var rb Runbook
	if err := yaml.Unmarshal([]byte(yamlContent), &rb); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	if err := Validate(&rb); err != nil {
		return nil, err
	}
	return &rb, nil
}

func validName(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			continue
		}
		return false
	}
	return true
}

func (rb *Runbook) EnvironmentAllowed(env string) bool {
	if len(rb.Spec.Targets.AllowedEnvironments) == 0 {
		return true
	}
	for _, e := range rb.Spec.Targets.AllowedEnvironments {
		if e == env || e == "*" {
			return true
		}
	}
	return false
}

func (rb *Runbook) NeedsApproval(env string) bool {
	if rb.Spec.Approval.Required && (rb.Spec.Approval.Environment == env || rb.Spec.Approval.Environment == "*" || rb.Spec.Approval.Environment == "") {
		return true
	}
	return false
}

func (rb *Runbook) RenderCommand(params map[string]string) string {
	cmd := rb.Spec.Execution.Command
	for k, v := range params {
		cmd = strings.ReplaceAll(cmd, "${"+k+"}", v)
	}
	return cmd
}
