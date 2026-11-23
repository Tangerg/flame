package delivery

var contract = buildContract()

func buildContract() *Registry {
	registry := newRegistry()
	registerLifecycle(registry)
	registerSessions(registry)
	registerRuns(registry)
	registerInterrupts(registry)
	registerPlan(registry)
	registerItems(registry)
	registerWorkspace(registry)
	registerRuntimeSubscription(registry)
	registerSkills(registry)
	registerAgentDocs(registry)
	registerMCP(registry)
	registerPlugins(registry)
	registerHooks(registry)
	registerApproval(registry)
	registerGoals(registry)
	registerProviders(registry)
	registerModels(registry)
	registerTools(registry)
	registerUsage(registry)
	registerFeedback(registry)
	return registry
}

func requires(features ...string) []CapabilityRule {
	return []CapabilityRule{{Requires: features}}
}
