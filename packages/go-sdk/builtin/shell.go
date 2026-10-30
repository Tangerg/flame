package builtin

type ShellArguments struct {
	Command                    string `json:"command" jsonschema:"minLength=1" jsonschema_description:"Shell command line, run by /bin/sh -c. Each call starts a fresh shell; directory changes, variables, and shell options do not persist."`
	Description                string `json:"description" jsonschema:"minLength=1,maxLength=120" jsonschema_description:"Concise action phrase shown while the command runs, such as Run backend tests. Describe the command's purpose; do not copy the command or predict its result."`
	TimeoutMillis              *int   `json:"timeout_millis,omitzero" jsonschema:"minimum=1" jsonschema_description:"Hard execution timeout in milliseconds. Omit for no hard timeout."`
	RunInBackground            bool   `json:"run_in_background,omitzero" jsonschema_description:"Return immediately with a shell_id while the command keeps running. Use for servers and watchers."`
	AutoBackgroundAfterSeconds *int   `json:"auto_background_after_seconds,omitzero" jsonschema:"minimum=1" jsonschema_description:"Move a foreground command to the background after this many seconds. Defaults to 60."`
}
