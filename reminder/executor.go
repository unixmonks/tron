package reminder

type PromptExecutor interface {
	ExecutePrompt(chatID, prompt string) (string, error)
}

type Executor struct {
	handler PromptExecutor
}

func NewExecutor(handler PromptExecutor) *Executor {
	return &Executor{
		handler: handler,
	}
}

func (e *Executor) Execute(recipient, prompt string) (string, error) {
	chatID := recipient
	if chatID == "" {
		chatID = "system:reminders"
	}
	return e.handler.ExecutePrompt(chatID, prompt)
}
