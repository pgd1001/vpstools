package runbooks

import (
	"fmt"
	"regexp"
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
	switch rb.Metadata.Risk {
	case RiskLow, RiskMedium, RiskHigh, RiskCritical:
	default:
		return &ValidationError{Field: "metadata.risk", Message: "must be low, medium, high, or critical"}
	}
	seen := make(map[string]bool, len(rb.Spec.Parameters))
	for i := range rb.Spec.Parameters {
		p := &rb.Spec.Parameters[i]
		if !validParameterName(p.Name) {
			return &ValidationError{Field: fmt.Sprintf("spec.parameters[%d].name", i), Message: "must contain only letters, numbers, and underscores"}
		}
		if seen[p.Name] {
			return &ValidationError{Field: "spec.parameters." + p.Name, Message: "is defined more than once"}
		}
		seen[p.Name] = true
		switch strings.ToLower(p.Type) {
		case "", "string", "integer", "number", "boolean", "enum":
		default:
			return &ValidationError{Field: "spec.parameters." + p.Name + ".type", Message: "must be string, integer, number, boolean, or enum"}
		}
		if len(p.AllowedValues) > 0 && strings.ToLower(p.Type) != "enum" {
			return &ValidationError{Field: "spec.parameters." + p.Name + ".allowedValues", Message: "requires type enum"}
		}
		if p.Default != "" {
			if err := validateParameterValue(*p, p.Default); err != nil {
				return &ValidationError{Field: "spec.parameters." + p.Name + ".default", Message: err.Error()}
			}
		}
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

var parameterToken = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// ValidateParams resolves defaults and validates all supplied values before a
// runbook is rendered. Unknown values are rejected so callers cannot smuggle
// unreviewed input into a command.
func (rb *Runbook) ValidateParams(params map[string]string) (map[string]string, error) {
	resolved := make(map[string]string, len(rb.Spec.Parameters))
	definitions := make(map[string]Parameter, len(rb.Spec.Parameters))
	for _, definition := range rb.Spec.Parameters {
		definitions[definition.Name] = definition
		if definition.Default != "" {
			resolved[definition.Name] = definition.Default
		}
	}
	for name := range params {
		if _, ok := definitions[name]; !ok {
			return nil, fmt.Errorf("unknown parameter %q", name)
		}
	}
	for name, value := range params {
		if err := validateParameterValue(definitions[name], value); err != nil {
			return nil, fmt.Errorf("parameter %q: %w", name, err)
		}
		resolved[name] = value
	}
	for _, definition := range rb.Spec.Parameters {
		if definition.Required && strings.TrimSpace(resolved[definition.Name]) == "" {
			return nil, fmt.Errorf("required parameter %q is missing", definition.Name)
		}
	}
	for _, token := range parameterToken.FindAllStringSubmatch(rb.Spec.Execution.Command, -1) {
		if _, ok := resolved[token[1]]; !ok {
			return nil, fmt.Errorf("command parameter %q is missing", token[1])
		}
	}
	return resolved, nil
}

// RenderCommand validates parameters and quotes each substitution for the
// POSIX shell used by the runner. It returns an error rather than rendering
// partially validated input.
func (rb *Runbook) RenderCommand(params map[string]string) (string, error) {
	resolved, err := rb.ValidateParams(params)
	if err != nil {
		return "", err
	}
	cmd := parameterToken.ReplaceAllStringFunc(rb.Spec.Execution.Command, func(token string) string {
		name := strings.TrimSuffix(strings.TrimPrefix(token, "${"), "}")
		return shellQuote(resolved[name])
	})
	return cmd, nil
}

func validParameterName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || (i > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func validateParameterValue(definition Parameter, value string) error {
	switch strings.ToLower(definition.Type) {
	case "", "string", "enum":
	case "integer":
		for i, r := range value {
			if (r < '0' || r > '9') && !(i == 0 && r == '-') {
				return fmt.Errorf("must be an integer")
			}
		}
		if value == "" || value == "-" {
			return fmt.Errorf("must be an integer")
		}
	case "number":
		dots := 0
		for i, r := range value {
			if r == '.' {
				dots++
				continue
			}
			if (r < '0' || r > '9') && !(i == 0 && r == '-') {
				return fmt.Errorf("must be a number")
			}
		}
		if value == "" || value == "-" || dots > 1 {
			return fmt.Errorf("must be a number")
		}
	case "boolean":
		if value != "true" && value != "false" {
			return fmt.Errorf("must be true or false")
		}
	}
	if len(definition.AllowedValues) > 0 {
		allowed := false
		for _, candidate := range definition.AllowedValues {
			if value == candidate {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("must be one of: %s", strings.Join(definition.AllowedValues, ", "))
		}
	}
	return nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
