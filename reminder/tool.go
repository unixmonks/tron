package reminder

import (
	"encoding/json"
	"fmt"
	"strings"

	"tron"
)

type ToolArgs struct {
	Action   string `json:"action"`
	Prompt   string `json:"prompt,omitempty"`
	Schedule string `json:"schedule,omitempty"`
	ID       int64  `json:"id,omitempty"`
}

type Tool struct {
	store     *Store
	scheduler *Scheduler
	chatID    string
}

func NewTool(store *Store, scheduler *Scheduler) *Tool {
	return &Tool{
		store:     store,
		scheduler: scheduler,
	}
}

func (t *Tool) SetContext(chatID string) {
	t.chatID = chatID
}

func (t *Tool) Definition() tron.Tool {
	return tron.Tool{
		Type: "function",
		Function: tron.ToolFunction{
			Name:        "reminder",
			Description: "Manage scheduled reminders that execute with full tool access. Reminders run at specified times and can check tasks, system status, or perform any other available action.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"action": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"list", "add", "delete", "enable", "disable", "run"},
						"description": "Action to perform: list (show all reminders), add (create new reminder), delete (remove reminder), enable/disable (toggle reminder), run (execute immediately)",
					},
					"prompt": map[string]interface{}{
						"type":        "string",
						"description": "The instruction to execute when the reminder fires (required for add). This prompt will be sent to the AI with full tool access.",
					},
					"schedule": map[string]interface{}{
						"type":        "string",
						"description": "Schedule format: 'daily:HH:MM' (e.g., daily:08:00), 'hourly:MM' (e.g., hourly:30), 'interval:DURATION' (e.g., interval:2h), 'cron:EXPR' (e.g., cron:0 8 * * 1-5), 'once:DATETIME' (e.g., once:2024-01-15T08:00)",
					},
					"id": map[string]interface{}{
						"type":        "integer",
						"description": "Reminder ID (required for delete, enable, disable, run)",
					},
				},
				"required": []string{"action"},
			},
		},
	}
}

func (t *Tool) Execute(argsJSON string) (string, error) {
	var args ToolArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}

	switch args.Action {
	case "list":
		return t.list()
	case "add":
		return t.add(args)
	case "delete":
		return t.delete(args.ID)
	case "enable":
		return t.setEnabled(args.ID, true)
	case "disable":
		return t.setEnabled(args.ID, false)
	case "run":
		return t.run(args.ID)
	default:
		return "", fmt.Errorf("unknown action: %s", args.Action)
	}
}

func (t *Tool) list() (string, error) {
	reminders, err := t.store.List()
	if err != nil {
		return "", err
	}

	if len(reminders) == 0 {
		return "No reminders configured.", nil
	}

	var sb strings.Builder
	sb.WriteString("**Reminders:**\n\n")

	for _, r := range reminders {
		status := "[ACTIVE]"
		if !r.Enabled {
			status = "[PAUSED]"
		}

		sb.WriteString(fmt.Sprintf("%s **[%d]**\n", status, r.ID))
		sb.WriteString(fmt.Sprintf("   Schedule: %s:%s\n", r.ScheduleType, r.ScheduleValue))
		sb.WriteString(fmt.Sprintf("   Next run: %s\n", r.NextRun.Format("2006-01-02 15:04 MST")))
		sb.WriteString(fmt.Sprintf("   Prompt: %s\n\n", truncate(r.Prompt, 100)))
	}

	return sb.String(), nil
}

func (t *Tool) add(args ToolArgs) (string, error) {
	if args.Prompt == "" {
		return "", fmt.Errorf("prompt is required")
	}
	if args.Schedule == "" {
		return "", fmt.Errorf("schedule is required")
	}

	scheduleType, scheduleValue, err := ParseSchedule(args.Schedule)
	if err != nil {
		return "", err
	}

	r := &Reminder{
		Prompt:        args.Prompt,
		ScheduleType:  scheduleType,
		ScheduleValue: scheduleValue,
		Recipient:     t.chatID,
		Enabled:       true,
	}

	if err := t.store.Create(r); err != nil {
		return "", err
	}

	return fmt.Sprintf("Created reminder (ID: %d)\nSchedule: %s:%s\nNext run: %s",
		r.ID, r.ScheduleType, r.ScheduleValue, r.NextRun.Format("2006-01-02 15:04 MST")), nil
}

func (t *Tool) delete(id int64) (string, error) {
	if id == 0 {
		return "", fmt.Errorf("id is required")
	}

	r, err := t.store.GetByID(id)
	if err != nil {
		return "", err
	}
	if r == nil {
		return "", fmt.Errorf("reminder %d not found", id)
	}

	if err := t.store.Delete(id); err != nil {
		return "", err
	}

	return fmt.Sprintf("Deleted reminder (ID: %d)", id), nil
}

func (t *Tool) setEnabled(id int64, enabled bool) (string, error) {
	if id == 0 {
		return "", fmt.Errorf("id is required")
	}

	r, err := t.store.GetByID(id)
	if err != nil {
		return "", err
	}
	if r == nil {
		return "", fmt.Errorf("reminder %d not found", id)
	}

	if err := t.store.SetEnabled(id, enabled); err != nil {
		return "", err
	}

	action := "enabled"
	if !enabled {
		action = "disabled"
	}

	return fmt.Sprintf("Reminder (ID: %d) has been %s", id, action), nil
}

func (t *Tool) run(id int64) (string, error) {
	if id == 0 {
		return "", fmt.Errorf("id is required")
	}

	if t.scheduler == nil {
		return "", fmt.Errorf("scheduler not available")
	}

	if err := t.scheduler.RunNow(id); err != nil {
		return "", err
	}

	return fmt.Sprintf("Reminder %d executed. Check for the result message.", id), nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
