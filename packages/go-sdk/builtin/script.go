package builtin

type ScriptArguments struct {
	Summary string   `json:"summary" jsonschema:"minLength=1,maxLength=80,pattern=^[^[:space:]](.*[^[:space:]])?$" jsonschema_description:"Concise 3-5 word label for what the script reads, at most 80 characters, without leading or trailing whitespace."`
	Script  string   `json:"script" jsonschema:"minLength=1,maxLength=65536" jsonschema_description:"JavaScript run as the body of an async function. Return the result the conversation needs."`
	Tools   []string `json:"tools" jsonschema:"minItems=1" jsonschema_description:"Exact names of the read-only tools the script calls."`
}
