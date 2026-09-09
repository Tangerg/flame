package agentmemory

import (
	"fmt"

	domain "github.com/Tangerg/flame/runtime/internal/domain/workspace/agentmemory"
)

func validateActiveTargetCatalog(items []domain.Item, scope domain.Scope, project string) error {
	return validateTargetCatalog(items, scope, project, activeStatus)
}

func validateManagementTargetCatalog(items []domain.Item, scope domain.Scope, project string) error {
	return validateTargetCatalog(items, scope, project, managementStatus)
}

func validateTargetCatalog(
	items []domain.Item,
	scope domain.Scope,
	project string,
	acceptStatus func(domain.Status) bool,
) error {
	if len(items) > domain.MaxVisiblePerTarget {
		return fmt.Errorf("agentmemory: %s target catalog exceeds %d items", scope, domain.MaxVisiblePerTarget)
	}
	seenIDs := make(map[domain.ItemID]struct{}, len(items))
	seenContent := make(map[string]struct{}, len(items))
	for index, item := range items {
		if err := item.ValidateFor(scope, project); err != nil {
			return fmt.Errorf("agentmemory: target catalog row %d is invalid: %w", index+1, err)
		}
		if !acceptStatus(item.Status) {
			return fmt.Errorf("agentmemory: target catalog row %q has hidden status %q", item.ID, item.Status)
		}
		if _, duplicate := seenIDs[item.ID]; duplicate {
			return fmt.Errorf("agentmemory: target catalog repeats item %q", item.ID)
		}
		seenIDs[item.ID] = struct{}{}
		digest := domain.Digest(item.Content)
		if _, duplicate := seenContent[digest]; duplicate {
			return fmt.Errorf("agentmemory: target catalog repeats content %q", digest)
		}
		seenContent[digest] = struct{}{}
	}
	return nil
}

func validateSearchCatalog(items []domain.Item, project string) error {
	byTarget := map[domain.Scope][]domain.Item{
		domain.ScopeProject: nil,
		domain.ScopeUser:    nil,
	}
	seen := make(map[domain.ItemID]struct{}, len(items))
	for index, item := range items {
		if item.Scope != domain.ScopeProject && item.Scope != domain.ScopeUser {
			return fmt.Errorf("agentmemory: search catalog row %d has unsupported scope %q", index+1, item.Scope)
		}
		if _, duplicate := seen[item.ID]; duplicate {
			return fmt.Errorf("agentmemory: search catalog repeats item %q", item.ID)
		}
		seen[item.ID] = struct{}{}
		byTarget[item.Scope] = append(byTarget[item.Scope], item)
	}
	for _, target := range []struct {
		scope   domain.Scope
		project string
	}{
		{scope: domain.ScopeProject, project: project},
		{scope: domain.ScopeUser},
	} {
		if err := validateActiveTargetCatalog(byTarget[target.scope], target.scope, target.project); err != nil {
			return fmt.Errorf("agentmemory: search catalog: %w", err)
		}
	}
	return nil
}

func activeStatus(status domain.Status) bool {
	return status == domain.StatusActive
}

func managementStatus(status domain.Status) bool {
	return status == domain.StatusActive || status == domain.StatusPending
}
