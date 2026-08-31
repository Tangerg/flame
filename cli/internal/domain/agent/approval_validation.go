package agent

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

func (a ApprovalMode) Validate() error {
	if !slices.Contains([]ApprovalMode{ApprovalModeSafe, ApprovalModeBalanced, ApprovalModeYolo}, a) {
		return fmt.Errorf("approval mode %q is invalid", a)
	}
	return nil
}

func (a ApprovalRule) Validate() error {
	if strings.TrimSpace(a.ID) == "" || strings.TrimSpace(a.Tool) == "" {
		return errors.New("approval rule: id and tool are required")
	}
	if !slices.Contains([]ApprovalRuleDecision{ApprovalRuleAllow, ApprovalRuleDeny}, a.Decision) {
		return fmt.Errorf("approval rule: decision %q is invalid", a.Decision)
	}
	if !slices.Contains([]RememberScope{RememberSession, RememberProject, RememberGlobal}, a.Scope) {
		return fmt.Errorf("approval rule: scope %q is invalid", a.Scope)
	}
	if a.Scope == RememberProject && strings.TrimSpace(a.Dir) == "" {
		return errors.New("approval rule: project scope requires a directory")
	}
	if a.Scope != RememberProject && a.Dir != "" {
		return errors.New("approval rule: only project scope may carry a directory")
	}
	return nil
}

func ValidateApprovalRules(rules []ApprovalRule) error {
	ids := make(map[string]struct{}, len(rules))
	for i, rule := range rules {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("approval rule %d: %w", i+1, err)
		}
		if _, duplicate := ids[rule.ID]; duplicate {
			return fmt.Errorf("approval rule id %q is duplicated", rule.ID)
		}
		ids[rule.ID] = struct{}{}
	}
	return nil
}

// ValidateApprovalRuleDeletion proves that an authoritative rule catalog no
// longer contains the exact identity passed to DeleteApprovalRule.
func ValidateApprovalRuleDeletion(rules []ApprovalRule, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("deleted approval rule id is empty")
	}
	if err := ValidateApprovalRules(rules); err != nil {
		return err
	}
	for _, rule := range rules {
		if rule.ID == id {
			return fmt.Errorf("approval rule %q remains after deletion", id)
		}
	}
	return nil
}
