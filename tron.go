package tron

import "context"

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type LLMResponse struct {
	Content   string
	ToolCalls []ToolCall
}

type IncomingMessage struct {
	Source           string
	SourceUUID       string
	SourceNumber     string
	SourceName       string
	Message          string
	Timestamp        int64
	GroupID          string
	IsGroup          bool
	ExpiresInSeconds int
}

type LLMClient interface {
	Chat(messages []Message, tools []Tool) (*LLMResponse, error)
}

type MemoryStore interface {
	AddMessage(chatID, role, content string, expiresInSeconds int) error
	GetHistory(chatID string) ([]Message, error)
	ClearHistory(chatID string) error
	Close() error
}

type PluginManager interface {
	Execute(name, argsJSON string) (string, error)
	ExecuteWithContext(name, argsJSON, chatID string) (string, error)
	GetTools() []Tool
	HasPlugin(name string) bool
	PluginCount() int
}

type SignalClient interface {
	SendMessage(recipient, message string) error
	SendGroupMessage(groupID, message string) error
	SubscribeMessages(ctx context.Context) <-chan IncomingMessage
}
