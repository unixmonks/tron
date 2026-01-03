package bot

import (
	"fmt"
	"log"
	"time"

	"tron"
)

type Handler struct {
	llm          tron.LLMClient
	plugins      tron.PluginManager
	memory       tron.MemoryStore
	systemPrompt string
	debug        bool
}

func NewHandler(llm tron.LLMClient, plugins tron.PluginManager, memory tron.MemoryStore, systemPrompt string, debug bool) *Handler {
	return &Handler{
		llm:          llm,
		plugins:      plugins,
		memory:       memory,
		systemPrompt: systemPrompt,
		debug:        debug,
	}
}

func (h *Handler) debugLog(format string, v ...interface{}) {
	if h.debug {
		log.Printf("[DEBUG] "+format, v...)
	}
}

func (h *Handler) HandleMessage(chatID, userMessage string, expiresInSeconds int) (string, error) {
	if err := h.memory.AddMessage(chatID, "user", userMessage, expiresInSeconds); err != nil {
		h.debugLog("Failed to save user message: %v", err)
	}

	history, err := h.memory.GetHistory(chatID)
	if err != nil {
		h.debugLog("Failed to get history: %v", err)
	}

	now := time.Now()
	dynamicPrompt := fmt.Sprintf("%s\n\nCurrent time: %s", h.systemPrompt, now.Format("2006-01-02 15:04:05 MST (Monday)"))

	messages := []tron.Message{
		{Role: "system", Content: dynamicPrompt},
	}
	messages = append(messages, history...)

	tools := h.plugins.GetTools()
	h.debugLog("User message: %s", userMessage)
	h.debugLog("History messages: %d", len(history))
	h.debugLog("Available tools: %d", len(tools))

	iteration := 0
	for {
		iteration++
		h.debugLog("Iteration %d - sending %d messages to LLM", iteration, len(messages))

		resp, err := h.llm.Chat(messages, tools)
		if err != nil {
			return "", fmt.Errorf("llm chat: %w", err)
		}

		if len(resp.ToolCalls) == 0 {
			h.debugLog("Final response: %s", resp.Content)

			if err := h.memory.AddMessage(chatID, "assistant", resp.Content, expiresInSeconds); err != nil {
				h.debugLog("Failed to save assistant message: %v", err)
			}

			return resp.Content, nil
		}

		h.debugLog("Got %d tool calls", len(resp.ToolCalls))

		messages = append(messages, tron.Message{
			Role:      "assistant",
			ToolCalls: resp.ToolCalls,
		})

		for _, tc := range resp.ToolCalls {
			h.debugLog("Tool call: %s(%s)", tc.Function.Name, tc.Function.Arguments)
			result := h.executeToolWithContext(tc.Function.Name, tc.Function.Arguments, chatID)
			h.debugLog("Tool result: %s", truncate(result, 200))
			messages = append(messages, tron.Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
			})
		}
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func (h *Handler) executeTool(name, argsJSON string) string {
	h.debugLog("Executing tool: %s with args: %s", name, argsJSON)

	result, err := h.plugins.Execute(name, argsJSON)
	if err != nil {
		return fmt.Sprintf("Error: %s", err)
	}

	return result
}

func (h *Handler) executeToolWithContext(name, argsJSON, chatID string) string {
	h.debugLog("Executing tool: %s with args: %s (chatID: %s)", name, argsJSON, chatID)

	result, err := h.plugins.ExecuteWithContext(name, argsJSON, chatID)
	if err != nil {
		return fmt.Sprintf("Error: %s", err)
	}

	return result
}

func (h *Handler) GenerateDailySummary() (string, error) {
	result, err := h.plugins.Execute("task", `{"action": "list"}`)
	if err != nil {
		result = fmt.Sprintf("Error getting tasks: %s", err)
	}

	return fmt.Sprintf("Good morning! Here's your daily summary:\n\n**Tasks:**\n%s", result), nil
}

func (h *Handler) ExecutePrompt(chatID, prompt string) (string, error) {
	return h.HandleMessage(chatID, prompt, 0)
}
