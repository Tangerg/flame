package protocol

// Every value except resync is also a subscribable topic; RuntimeTopic derives
// from these so the two tables cannot disagree.
type RuntimeEventType string

const (
	RuntimeFilesChanged      RuntimeEventType = "files.changed"
	RuntimeSkillsChanged     RuntimeEventType = "skills.changed"
	RuntimeMCPChanged        RuntimeEventType = "mcp.changed"
	RuntimePluginsChanged    RuntimeEventType = "plugins.changed"
	RuntimePluginDataChanged RuntimeEventType = "plugins.dataChanged"
	RuntimeSessionsChanged   RuntimeEventType = "sessions.changed"
	RuntimeRunsChanged       RuntimeEventType = "runs.changed"
	RuntimePlanChanged       RuntimeEventType = "plan.changed"
	RuntimeGoalsChanged      RuntimeEventType = "goals.changed"
	RuntimeInterruptsChanged RuntimeEventType = "interrupts.changed"
	RuntimeHooksChanged      RuntimeEventType = "hooks.changed"
	RuntimeModelsChanged     RuntimeEventType = "models.changed"
	RuntimeApprovalsChanged  RuntimeEventType = "approvals.changed"
	RuntimeResync            RuntimeEventType = "resync"
)

type RuntimeTopic string

const (
	TopicFilesChanged      = RuntimeTopic(RuntimeFilesChanged)
	TopicSkillsChanged     = RuntimeTopic(RuntimeSkillsChanged)
	TopicMCPChanged        = RuntimeTopic(RuntimeMCPChanged)
	TopicPluginsChanged    = RuntimeTopic(RuntimePluginsChanged)
	TopicPluginDataChanged = RuntimeTopic(RuntimePluginDataChanged)
	TopicSessionsChanged   = RuntimeTopic(RuntimeSessionsChanged)
	TopicRunsChanged       = RuntimeTopic(RuntimeRunsChanged)
	TopicPlanChanged       = RuntimeTopic(RuntimePlanChanged)
	TopicGoalsChanged      = RuntimeTopic(RuntimeGoalsChanged)
	TopicInterruptsChanged = RuntimeTopic(RuntimeInterruptsChanged)
	TopicHooksChanged      = RuntimeTopic(RuntimeHooksChanged)
	TopicModelsChanged     = RuntimeTopic(RuntimeModelsChanged)
	TopicApprovalsChanged  = RuntimeTopic(RuntimeApprovalsChanged)
)

func RuntimeTopics() []RuntimeTopic {
	return []RuntimeTopic{
		TopicFilesChanged, TopicSkillsChanged, TopicMCPChanged, TopicPluginsChanged, TopicPluginDataChanged,
		TopicSessionsChanged, TopicRunsChanged, TopicPlanChanged, TopicGoalsChanged,
		TopicInterruptsChanged, TopicHooksChanged,
		TopicModelsChanged, TopicApprovalsChanged,
	}
}

const (
	MaxSubscriptionTopics  = 32
	MaxSubscriptionWatches = 32
)

type RuntimeSubscribeRequest struct {
	Topics  []RuntimeTopic `json:"topics"`
	Watches []WatchSpec    `json:"watches,omitempty"`
}

type WatchSpec struct {
	WatchID   string       `json:"watchId"`
	Workspace WorkspaceRef `json:"workspace"`
	Paths     []string     `json:"paths,omitempty"`
}

type RuntimeSubscribeResponse struct{}

type RuntimeEventNotification struct {
	Event RuntimeEvent `json:"event"`
}

// The largest sequence that stays an exact integer after JSON decoding in
// JavaScript; an imprecise sequence would make gap detection lie.
const MaximumRuntimeEventSequence = MaximumExactJSONInteger

// Every variant is an invalidation, not a payload: a status or count here would be
// a second source of truth that drifts once a frame is dropped.
type RuntimeEvent struct {
	Type     RuntimeEventType `json:"type"`
	Sequence uint64           `json:"sequence"`

	WatchID    string         `json:"watchId,omitempty"`
	Workspace  *WorkspaceRef  `json:"workspace,omitzero"`
	Paths      []string       `json:"paths,omitempty"`
	Names      []string       `json:"names,omitempty"`
	Servers    []MCPServerID  `json:"servers,omitempty"`
	SessionIDs []string       `json:"sessionIds,omitempty"`
	RunIDs     []string       `json:"runIds,omitempty"`
	Topics     []RuntimeTopic `json:"topics,omitempty"`
	WatchIDs   []string       `json:"watchIds,omitempty"`
}
