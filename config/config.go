package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

type Config struct {
	SignalCLIURL      string `yaml:"signal_cli_url"`
	SignalBotAccount  string `yaml:"signal_bot_account"`
	SignalOperator    string `yaml:"signal_operator"`
	LLMAPIURL         string `yaml:"llm_api_url"`
	LLMAPIKey         string `yaml:"llm_api_key"`
	LLMModel          string `yaml:"llm_model"`
	LLMSystemPrompt   string `yaml:"llm_system_prompt"`
	PluginDir         string `yaml:"plugin_dir"`
	DBPath            string `yaml:"db_path"`
	TriggerKeyword    string `yaml:"trigger_keyword"`
	MemoryMaxMessages int    `yaml:"memory_max_messages"`
	MemoryMaxMinutes  int    `yaml:"memory_max_minutes"`
	DailySummaryHour  int    `yaml:"daily_summary_hour"`
	Debug             bool   `yaml:"-"`
}

const defaultSystemPrompt = `You are a helpful personal assistant bot on Signal. You can manage tasks and answer general questions.

Be concise - responses go to a mobile chat. Use the available tools to help the user. Never use emojis.`

func Load(configPath string, debug bool) (*Config, error) {
	cfg := &Config{
		SignalCLIURL:      "http://localhost:8080",
		LLMAPIURL:         "https://api.deepinfra.com/v1/openai",
		LLMModel:          "deepseek-ai/DeepSeek-V3.1",
		LLMSystemPrompt:   defaultSystemPrompt,
		PluginDir:         "plugins.d",
		DBPath:            "tron.db",
		TriggerKeyword:    "T",
		MemoryMaxMessages: 50,
		MemoryMaxMinutes:  60,
		DailySummaryHour:  7,
		Debug:             debug,
	}

	if configPath != "" {
		if err := cfg.loadFromYAML(configPath); err != nil {
			return nil, fmt.Errorf("load config file: %w", err)
		}
	}

	cfg.applyEnvOverrides()

	if cfg.SignalBotAccount == "" {
		return nil, fmt.Errorf("signal_bot_account is required (set via config file or SIGNAL_BOT_ACCOUNT env var)")
	}
	if cfg.SignalOperator == "" {
		return nil, fmt.Errorf("signal_operator is required (set via config file or SIGNAL_OPERATOR env var)")
	}
	if cfg.LLMAPIKey == "" {
		return nil, fmt.Errorf("llm_api_key is required (set via config file or LLM_API_KEY env var)")
	}

	return cfg, nil
}

func (c *Config) loadFromYAML(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, c)
}

func (c *Config) applyEnvOverrides() {
	if v := os.Getenv("SIGNAL_CLI_URL"); v != "" {
		c.SignalCLIURL = v
	}
	if v := os.Getenv("SIGNAL_BOT_ACCOUNT"); v != "" {
		c.SignalBotAccount = v
	}
	if v := os.Getenv("SIGNAL_OPERATOR"); v != "" {
		c.SignalOperator = v
	}
	if v := os.Getenv("LLM_API_URL"); v != "" {
		c.LLMAPIURL = v
	}
	if v := os.Getenv("LLM_API_KEY"); v != "" {
		c.LLMAPIKey = v
	}
	if v := os.Getenv("LLM_MODEL"); v != "" {
		c.LLMModel = v
	}
	if v := os.Getenv("LLM_SYSTEM_PROMPT"); v != "" {
		c.LLMSystemPrompt = v
	}
	if v := os.Getenv("PLUGIN_DIR"); v != "" {
		c.PluginDir = v
	}
	if v := os.Getenv("DB_PATH"); v != "" {
		c.DBPath = v
	}
	if v := os.Getenv("TRIGGER_KEYWORD"); v != "" {
		c.TriggerKeyword = v
	}
	if v := os.Getenv("MEMORY_MAX_MESSAGES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.MemoryMaxMessages = n
		}
	}
	if v := os.Getenv("MEMORY_MAX_MINUTES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.MemoryMaxMinutes = n
		}
	}
	if v := os.Getenv("DAILY_SUMMARY_HOUR"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.DailySummaryHour = n
		}
	}
}
