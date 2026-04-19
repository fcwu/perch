## MODIFIED Requirements

### Requirement: OpenCode subagent launch mode

The `AgentRuntime` for OpenCode SHALL support a `RunAgent(agentName, prompt, workdir string) (cmd, args)` helper that returns the command to launch OpenCode in non-interactive subagent mode.

#### Scenario: launch as-query agent with a prompt
- **WHEN** `UserSessionManager` starts a new session for a user query
- **THEN** it SHALL use `AgentRuntime.RunAgent("as-query", userQuery, workdir)` to obtain the command, which resolves to: `opencode run --agent as-query "<userQuery>"`

#### Scenario: subagent exits after completion
- **WHEN** the OpenCode subagent finishes processing the query
- **THEN** the PTY process SHALL exit with code 0; the server SHALL detect EOF and trigger session completion

#### Scenario: invalid agent name
- **WHEN** `RunAgent` is called with an agent name that does not exist in `.opencode/agents/`
- **THEN** OpenCode SHALL exit with a non-zero code; the server SHALL surface an error message to the user's WebSocket
