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
	"tron/scheduler"
	signalcli "tron/signal"
)

type app struct {
	cfg             *config.Config
	signalClient    *signalcli.Client
	handler         *bot.Handler
	memoryStore     *memory.Store
	sched           *scheduler.Scheduler
	operatorAddress string
}

func main() {
	debug := flag.Bool("debug", false, "Enable debug logging")
	configPath := flag.String("config", "", "Path to YAML config file")
	flag.Parse()
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	cfg, err := config.Load(*configPath, *debug)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	logConfig(cfg)

	a, cleanup, err := newApp(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize: %v", err)
	}
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go a.sched.Start(ctx)

	a.run(ctx, cancel)
}

func logConfig(cfg *config.Config) {
	log.Printf("Starting Signal bot...")
	log.Printf("  Bot account: %s", cfg.SignalBotAccount)
	log.Printf("  Operator: %s", cfg.SignalOperator)
	log.Printf("  LLM: %s @ %s", cfg.LLMModel, cfg.LLMAPIURL)
	log.Printf("  Plugin dir: %s", cfg.PluginDir)
	log.Printf("  Database: %s", cfg.DBPath)
	log.Printf("  Trigger keyword: %s", cfg.TriggerKeyword)
	log.Printf("  Memory: %d messages, %d minutes", cfg.MemoryMaxMessages, cfg.MemoryMaxMinutes)
	log.Printf("  Daily summary: %02d:00 PDT", cfg.DailySummaryHour)
}

func newApp(cfg *config.Config) (*app, func(), error) {
	signalClient := signalcli.NewClient(cfg.SignalCLIURL, cfg.SignalBotAccount)
	llmClient := llm.NewClient(cfg.LLMAPIURL, cfg.LLMAPIKey, cfg.LLMModel)

	memoryStore, err := memory.NewStore(cfg.DBPath, cfg.MemoryMaxMessages, cfg.MemoryMaxMinutes)
	if err != nil {
		return nil, nil, err
	}

	pluginManager, err := plugins.NewManager(cfg.PluginDir, cfg.Debug)
	if err != nil {
		memoryStore.Close()
		return nil, nil, err
	}
	log.Printf("  Plugins loaded: %d", pluginManager.PluginCount())

	handler := bot.NewHandler(llmClient, pluginManager, memoryStore, cfg.LLMSystemPrompt, cfg.Debug)

	a := &app{
		cfg:          cfg,
		signalClient: signalClient,
		handler:      handler,
		memoryStore:  memoryStore,
	}

	sched, err := scheduler.NewScheduler(cfg.DailySummaryHour, handler.GenerateDailySummary, a.sendToOperator)
	if err != nil {
		memoryStore.Close()
		return nil, nil, err
	}
	a.sched = sched

	cleanup := func() { memoryStore.Close() }
	return a, cleanup, nil
}

func (a *app) sendToOperator(message string) error {
	addr := a.operatorAddress
	if addr == "" {
		addr = formatRecipient(a.cfg.SignalOperator)
	}
	return a.signalClient.SendMessage(addr, message)
}

func (a *app) run(ctx context.Context, cancel context.CancelFunc) {
	messages := a.signalClient.SubscribeMessages(ctx)

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
			a.handleMessage(msg)
		}
	}
}

func (a *app) handleMessage(msg tron.IncomingMessage) {
	log.Printf("Message from: source=%s uuid=%s number=%s name=%s group=%v",
		msg.Source, msg.SourceUUID, msg.SourceNumber, msg.SourceName, msg.IsGroup)

	if !isOperator(msg, a.cfg.SignalOperator) {
		log.Printf("Ignoring message from non-operator")
		return
	}

	userMessage := msg.Message
	var chatID string

	if msg.IsGroup {
		if !strings.HasPrefix(userMessage, a.cfg.TriggerKeyword+" ") {
			log.Printf("Ignoring group message without trigger keyword")
			return
		}
		userMessage = strings.TrimPrefix(userMessage, a.cfg.TriggerKeyword+" ")
		chatID = "group:" + msg.GroupID
	} else {
		if a.operatorAddress == "" {
			a.operatorAddress = resolveAddress(msg)
			log.Printf("Operator address set to: %s", a.operatorAddress)
		}
		chatID = "dm:" + a.operatorAddress
	}

	log.Printf("Received message (chat=%s, expires=%ds): %s", chatID, msg.ExpiresInSeconds, userMessage)

	response, err := a.handler.HandleMessage(chatID, userMessage, msg.ExpiresInSeconds)
	if err != nil {
		log.Printf("Error handling message: %v", err)
		response = "Sorry, I encountered an error processing your request."
	}

	if msg.IsGroup {
		if err := a.signalClient.SendGroupMessage(msg.GroupID, response); err != nil {
			log.Printf("Error sending group response: %v", err)
		}
	} else {
		if err := a.sendToOperator(response); err != nil {
			log.Printf("Error sending response: %v", err)
		}
	}
}

func resolveAddress(msg tron.IncomingMessage) string {
	if msg.SourceUUID != "" {
		return msg.SourceUUID
	}
	if msg.SourceNumber != "" {
		return msg.SourceNumber
	}
	return msg.Source
}

func formatRecipient(account string) string {
	if strings.HasPrefix(account, "+") || strings.HasPrefix(account, "u:") {
		return account
	}
	return "u:" + account
}

func isOperator(msg tron.IncomingMessage, operator string) bool {
	operator = strings.TrimPrefix(operator, "+")
	operator = strings.TrimPrefix(operator, "u:")
	operator = strings.ToLower(operator)

	for _, c := range []string{msg.Source, msg.SourceUUID, msg.SourceNumber, msg.SourceName} {
		c = strings.TrimPrefix(c, "+")
		c = strings.TrimPrefix(c, "u:")
		if strings.ToLower(c) == operator {
			return true
		}
	}
	return false
}
