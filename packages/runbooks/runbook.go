package runbooks

type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

type Runbook struct {
	APIVersion  string   `yaml:"apiVersion" json:"apiVersion"`
	Kind        string   `yaml:"kind"        json:"kind"`
	Metadata    Metadata `yaml:"metadata"    json:"metadata"`
	Spec        Spec     `yaml:"spec"        json:"spec"`
}

type Metadata struct {
	Name        string `yaml:"name"        json:"name"`
	Title       string `yaml:"title"       json:"title"`
	Description string `yaml:"description" json:"description"`
	Risk        RiskLevel `yaml:"risk"     json:"risk"`
	Tags        []string  `yaml:"tags"     json:"tags"`
	Version     int       `yaml:"version"  json:"version"`
}

type Spec struct {
	Parameters []Parameter  `yaml:"parameters" json:"parameters"`
	Targets    TargetRules  `yaml:"targets"    json:"targets"`
	Approval   ApprovalRule `yaml:"approval"   json:"approval"`
	Execution  Execution    `yaml:"execution"  json:"execution"`
	Output     Output       `yaml:"output"     json:"output"`
	Rollback   Rollback     `yaml:"rollback"   json:"rollback"`
}

type Parameter struct {
	Name         string   `yaml:"name"         json:"name"`
	Type         string   `yaml:"type"         json:"type"`
	Default      string   `yaml:"default"      json:"default"`
	AllowedValues []string `yaml:"allowedValues" json:"allowedValues"`
}

type TargetRules struct {
	AllowedTags         map[string]string `yaml:"allowedTags"         json:"allowedTags"`
	AllowedEnvironments []string          `yaml:"allowedEnvironments" json:"allowedEnvironments"`
}

type ApprovalRule struct {
	Rules []struct {
		When struct {
			Environment string `yaml:"environment" json:"environment"`
		} `yaml:"when" json:"when"`
		Required bool `yaml:"required" json:"required"`
	} `yaml:"rules" json:"rules"`
}

type Execution struct {
	Shell          string `yaml:"shell"           json:"shell"`
	Sudo           bool   `yaml:"sudo"            json:"sudo"`
	TimeoutSeconds int    `yaml:"timeoutSeconds"  json:"timeoutSeconds"`
	Concurrency    int    `yaml:"concurrency"     json:"concurrency"`
	Command        string `yaml:"command"         json:"command"`
}

type Output struct {
	Store  string   `yaml:"store"  json:"store"`
	Redact []string `yaml:"redact" json:"redact"`
}

type Rollback struct {
	Notes string `yaml:"notes" json:"notes"`
}
