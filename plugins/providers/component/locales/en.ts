export const en: Record<string, string> = {
  "providers.apiKey.replace": "Replace API key…",
  "providers.apiKey.clear": "Remove stored key",
  "providers.apiKey.clear.title": "Remove the stored key?",
  "providers.apiKey.clear.body":
    "{{provider}} loses its saved API key. You'll need to paste it again to reconnect.",
  "providers.apiKey.placeholder": "API key",
  "providers.apiKey.envPlaceholder": "Using environment key — type to override",
  "providers.apiKey.aria": "{{provider}} API key",
  "providers.baseUrl.placeholder": "Base URL",
  "providers.baseUrl.aria": "{{provider}} base URL",
  "providers.key": "key {{masked}}",
  "providers.ready": "ready",
  "providers.fromEnv": "from env",
  "providers.notConfigured": "not configured",
  "providers.save": "Save",
  "providers.saving": "Saving…",
  "providers.test": "Test",
  "providers.testing": "Testing…",
  "providers.connectionOk": "Connection OK",
  "providers.error.save": "Save failed",
  "providers.error.test": "Test failed",
  "providers.models.error": "Couldn't load models.",
  "providers.models.refresh": "Refresh models",
  "providers.empty": "No providers",
  "providers.empty.sub": "The runtime reports no supported LLM providers.",
  "providers.utility.title": "Utility model",
  "providers.utility.desc":
    "Model for summaries, titles, and fact extraction. Uses the Runtime default when unset. Changes apply to the next utility call, including in an existing run.",
  "providers.utility.main": "Use Runtime default",
  "providers.utility.error": "Couldn't set the utility model.",
  "providers.embedding.title": "Embedding model",
  "providers.embedding.desc":
    "Optionally improves Agent Memory ranking. Pick an embedding-capable provider; when Off, Memory continues with keyword matching. Applies to subsequent searches.",
  "providers.embedding.off": "Off",
  "providers.embedding.none":
    "No embedding-capable provider configured yet — add one (e.g. OpenAI) above.",
  "providers.embedding.error": "Couldn't set the embedding model.",
  "providers.testOutcome.notConfigured":
    "Finish configuring the provider first.",
  "providers.testOutcome.failed":
    "The provider couldn't be reached, or it rejected the test request.",
  "providers.apiKey.label": "API key",
  "providers.baseUrl.label": "Base URL",
  "providers.baseUrl.labelRequired": "Base URL · required",
  "providers.baseUrl.required":
    "This provider needs a base URL before it can be saved.",
  "providers.saveAndTest": "Save & test",
  "providers.saved": "Saved",
  "providers.role.loadError":
    "Couldn’t load this setting, so its current value is unknown.",
  "providers.role.stale":
    "Showing the last known value — refreshing it failed.",
  "providers.setup.steps":
    "Pick one provider, paste its API key, then Save & test. That is all you need to start.",
  "providers.setup.done":
    "A provider is ready. Close Settings to return — your draft is still there.",
  "providers.roles.heading": "Model roles",
  "rpcError.invalid_api_key":
    "The provider rejected the API key — check it in provider settings.",
  "rpcError.timeout":
    "The request to the model provider timed out — check your connection and retry.",
  title: "Providers",
  cancel: "Cancel",
  reading: "Loading",
  refresh: "Refresh",
  retry: "Retry",
  "command.uncertain":
    "The original command outcome is not confirmed. Reconcile it before issuing another command or changing catalogs.",
  "command.reconcile": "Reconcile original command",
  "read.retained": "The last accepted read is retained.",
  close: "Close",
  "providers.embedding.noPreset":
    "No preset embedding model is available for the configured providers.",
};
