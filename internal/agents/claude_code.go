package agents

import llm "github.com/openpaw/openpaw/internal/llm"

// Type aliases — canonical definitions live in internal/llm
type ClaudeUsage = llm.ClaudeUsage
type StreamEvent = llm.StreamEvent

const (
	EventTextDelta    = llm.EventTextDelta
	EventToolStart    = llm.EventToolStart
	EventToolDelta    = llm.EventToolDelta
	EventToolEnd      = llm.EventToolEnd
	EventTurnComplete = llm.EventTurnComplete
	EventResult       = llm.EventResult
	EventError        = llm.EventError
	EventInit         = llm.EventInit
)

var BuilderTools = []string{
	"Read", "Edit", "Write", "Bash", "Grep", "Glob",
	"NotebookEdit", "WebFetch", "WebSearch",
}
