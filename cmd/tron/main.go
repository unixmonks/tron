package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"tron"
	"tron/bot"
	"tron/config"
	"tron/llm"
	"tron/memory"
	"tron/plugins"
	"tron/reminder"
	"tron/scheduler"
	signalcli "tron/signal"
)

func main() {
	debug := flag.Bool("debug", false, "Enable debug logging")
	configPath := flag.String("config", "", "Path to YAML config file")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lshortfile)

	cfg, err := config.Load(*configPath, *debug)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Starting Signal bot...")
	log.Printf("  Bot account: %s", cfg.SignalBotAccount)
	log.Printf("  Operator: %s", cfg.SignalOperator)
	log.Printf("  LLM: %s @ %s", cfg.LLMModel, cfg.LLMAPIURL)
	log.Printf("  Plugin dir: %s", cfg.PluginDir)
	log.Printf("  Database: %s", cfg.DBPath)
	log.Printf("  Trigger keyword: %s", cfg.TriggerKeyword)
	log.Printf("  Memory: %d messages, %d minutes", cfg.MemoryMaxMessages, cfg.MemoryMaxMinutes)
	log.Printf("  Daily summary: %02d:00 PDT", cfg.DailySummaryHour)

	signalClient := signalcli.NewClient(cfg.SignalCLIURL, cfg.SignalBotAccount)
	llmClient := llm.NewClient(cfg.LLMAPIURL, cfg.LLMAPIKey, cfg.LLMModel)

	memoryStore, err := memory.NewStore(cfg.DBPath, cfg.MemoryMaxMessages, cfg.MemoryMaxMinutes)
	if err != nil {
		log.Fatalf("Failed to open memory store: %v", err)
	}
	defer memoryStore.Close()

	pluginManager, err := plugins.NewManager(cfg.PluginDir, cfg.Debug)
	if err != nil {
		log.Fatalf("Failed to load plugins: %v", err)
	}
	log.Printf("  Plugins loaded: %d", pluginManager.PluginCount())

	handler := bot.NewHandler(llmClient, pluginManager, memoryStore, cfg.LLMSystemPrompt, cfg.Debug)

	var operatorAddress string

	sendToOperator := func(message string) error {
		addr := operatorAddress
		if addr == "" {
			addr = formatRecipient(cfg.SignalOperator)
		}
		return signalClient.SendMessage(addr, message)
	}

	sendToRecipient := func(recipient, message string) error {
		if recipient == "" {
			return sendToOperator(message)
		}

		if strings.HasPrefix(recipient, "group:") {
			groupID := strings.TrimPrefix(recipient, "group:")
			return signalClient.SendGroupMessage(groupID, message)
		}

		if strings.HasPrefix(recipient, "dm:") {
			addr := strings.TrimPrefix(recipient, "dm:")
			return signalClient.SendMessage(addr, message)
		}

		return sendToOperator(message)
	}

	reminderStore, err := reminder.NewStore(memoryStore.DB())
	if err != nil {
		log.Fatalf("Failed to create reminder store: %v", err)
	}

	reminderExecutor := reminder.NewExecutor(handler)
	reminderScheduler := reminder.NewScheduler(reminderStore, reminderExecutor.Execute, sendToRecipient, cfg.Debug)

	reminderTool := reminder.NewTool(reminderStore, reminderScheduler)
	pluginManager.RegisterTool("reminder", reminderTool)
	log.Printf("  Reminder system: enabled")

	sched, err := scheduler.NewScheduler(cfg.DailySummaryHour, handler.GenerateDailySummary, sendToOperator)
	if err != nil {
		log.Fatalf("Failed to create scheduler: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sched.Start(ctx)
	go reminderScheduler.Start(ctx)

	messages := signalClient.SubscribeMessages(ctx)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Println("Bot is running. Waiting for messages...")

	for {
		select {
		case <-sigChan:
			log.Println("Shutting down...")
			cancel()
			return

		case msg, ok := <-messages:
			if !ok {
				log.Println("Message channel closed")
				return
			}

			log.Printf("Message from: source=%s uuid=%s number=%s name=%s group=%v",
				msg.Source, msg.SourceUUID, msg.SourceNumber, msg.SourceName, msg.IsGroup)

			if !isOperator(msg, cfg.SignalOperator) {
				log.Printf("Ignoring message from non-operator")
				continue
			}

			userMessage := msg.Message
			var chatID string

			if msg.IsGroup {
				if !strings.HasPrefix(userMessage, cfg.TriggerKeyword+" ") {
					log.Printf("Ignoring group message without trigger keyword")
					continue
				}
				userMessage = strings.TrimPrefix(userMessage, cfg.TriggerKeyword+" ")
				chatID = "group:" + msg.GroupID
			} else {
				if operatorAddress == "" {
					if msg.SourceUUID != "" {
						operatorAddress = msg.SourceUUID
					} else if msg.SourceNumber != "" {
						operatorAddress = msg.SourceNumber
					} else {
						operatorAddress = msg.Source
					}
					log.Printf("Operator address set to: %s", operatorAddress)
				}
				chatID = "dm:" + operatorAddress
			}

			log.Printf("Received message (chat=%s, expires=%ds): %s", chatID, msg.ExpiresInSeconds, userMessage)

			response, err := handler.HandleMessage(chatID, userMessage, msg.ExpiresInSeconds)
			if err != nil {
				log.Printf("Error handling message: %v", err)
				response = "Sorry, I encountered an error processing your request."
			}

			if msg.IsGroup {
				if err := signalClient.SendGroupMessage(msg.GroupID, response); err != nil {
					log.Printf("Error sending group response: %v", err)
				}
			} else {
				if err := sendToOperator(response); err != nil {
					log.Printf("Error sending response: %v", err)
				}
			}
		}
	}
}

func formatRecipient(account string) string {
	if strings.HasPrefix(account, "+") {
		return account
	}
	if strings.HasPrefix(account, "u:") {
		return account
	}
	return "u:" + account
}

func isOperator(msg tron.IncomingMessage, operator string) bool {
	operator = strings.TrimPrefix(operator, "+")
	operator = strings.TrimPrefix(operator, "u:")
	operator = strings.ToLower(operator)

	candidates := []string{
		msg.Source,
		msg.SourceUUID,
		msg.SourceNumber,
		msg.SourceName,
	}

	for _, c := range candidates {
		c = strings.TrimPrefix(c, "+")
		c = strings.TrimPrefix(c, "u:")
		if strings.ToLower(c) == operator {
			return true
		}
	}

	return false
}
