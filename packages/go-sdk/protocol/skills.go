package protocol

import "github.com/Tangerg/flame/packages/go-sdk/skill"

//flame:enum
type SkillScope string

const (
	SkillScopeRepository   SkillScope = SkillScope(skill.Repository)
	SkillScopeUser         SkillScope = SkillScope(skill.User)
	SkillScopeInstallation SkillScope = SkillScope(skill.Installation)
)

type SkillInstallation struct {
	InstallationID string `json:"installationId"`
	Digest         string `json:"digest"`
}

type Skill struct {
	Installation *SkillInstallation `json:"installation,omitzero"`
	Name         string             `json:"name"`
	Description  string             `json:"description,omitempty"`
	Scope        SkillScope         `json:"scope"`
}

//flame:enum
type SkillLifecycle string

const (
	SkillLifecycleActive   SkillLifecycle = "active"
	SkillLifecycleArchived SkillLifecycle = "archived"
)

type ManagedSkill struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Lifecycle   SkillLifecycle `json:"lifecycle"`
}

type SkillNameRequest struct {
	Name string `json:"name"`
}

type SkillDiscovery struct {
	Skills      []Skill           `json:"skills"`
	Diagnostics []SkillDiagnostic `json:"diagnostics"`
}

type SkillDiagnostic struct {
	Name   string `json:"name"`
	Detail string `json:"detail"`
}

type SkillDetailRequest struct {
	Workspace WorkspaceRef `json:"workspace"`
	Name      string       `json:"name"`
}

type SkillDetail struct {
	Skill
	Path         string `json:"path"`
	Revision     string `json:"revision"`
	Instructions string `json:"instructions"`
}
