# Tron

A Signal messaging bot with LLM integration and plugin support.

## Prerequisites

- Go 1.25.3+
- [signal-cli](https://github.com/AsamK/signal-cli) running in JSON-RPC daemon mode
- An LLM API endpoint (OpenAI-compatible, e.g., DeepInfra, OpenAI, local Ollama)
- SQLite3 (for conversation memory and reminders)
- Taskwarrior (optional, for task plugin)

### Setting up signal-cli

1. Install signal-cli from https://github.com/AsamK/signal-cli
2. Register or link your bot account
3. Run in JSON-RPC daemon mode:

```bash
signal-cli -a +1234567890 daemon --http=localhost:8080
```

The bot connects to the JSON-RPC endpoint for sending messages and subscribes to SSE events for receiving them.

## Building

```bash
# Build the binary
make build

# Build with formatting, vetting, and tests
make all

# Clean build artifacts
make clean
```

## Running

```bash
# Source environment config
source .env

# Build and run
make run

# Run with debug logging
make run-debug
```

## Configuration

Configuration can be done via YAML file, environment variables, or both. Environment variables take precedence over YAML values.

**Priority:** Defaults → YAML file → Environment variables (highest)

### YAML Config File

Copy the example config and customize:

```bash
cp config.example.yaml config.yaml
```

Run with the config file:

```bash
./bin/tron -config config.yaml
```

Example `config.yaml`:

```yaml
# Signal Configuration
signal_cli_url: "http://localhost:8080"
signal_bot_account: "+1234567890"
signal_operator: "+0987654321"

# LLM Configuration
llm_api_url: "https://api.deepinfra.com/v1/openai"
llm_api_key: "your-api-key-here"
llm_model: "deepseek-ai/DeepSeek-V3.1"
llm_system_prompt: |
  You are a personal assistant bot on Signal.
  Keep responses short. Never use emojis.

# Storage
plugin_dir: "plugins.d"
db_path: "tron.db"

# Behavior
trigger_keyword: "T"
memory_max_messages: 50
memory_max_minutes: 60
daily_summary_hour: 7
```

### Environment Variables

You can also use environment variables (useful for secrets or overriding config):

```bash
# Required (if not in YAML)
export SIGNAL_BOT_ACCOUNT="+1234567890"
export SIGNAL_OPERATOR="your-uuid-or-number"
export LLM_API_KEY="your-api-key"

# Optional
export SIGNAL_CLI_URL="http://localhost:8080"
export LLM_API_URL="https://api.deepinfra.com/v1/openai"
export LLM_MODEL="deepseek-ai/DeepSeek-V3.1"
export LLM_SYSTEM_PROMPT="You are a helpful assistant..."
export PLUGIN_DIR="plugins.d"
export DB_PATH="tron.db"
export TRIGGER_KEYWORD="T"
export MEMORY_MAX_MESSAGES="50"
export MEMORY_MAX_MINUTES="60"
export DAILY_SUMMARY_HOUR="7"
```

### Mixed Usage

A common pattern is to put non-sensitive config in YAML and secrets in environment variables:

```bash
# Keep API key out of config file
export LLM_API_KEY="your-api-key"

# Run with YAML config (env var overrides any llm_api_key in YAML)
./bin/tron -config config.yaml
```

## Plugins

Tron supports external plugins (shell scripts, Python, etc.) and internal tools (Go-based).

See [PLUGINS.md](PLUGINS.md) for:
- Using included plugins (`task`, `ps`)
- Creating custom plugins
- Plugin configuration

## Reminders

The reminder system allows scheduling prompts that execute with full tool access. The scheduler checks for due reminders every minute.

### Schedule Formats

| Format | Example | Description |
|--------|---------|-------------|
| `daily:HH:MM` | `daily:08:00` | Run daily at specified time |
| `hourly:MM` | `hourly:30` | Run every hour at specified minute |
| `interval:DURATION` | `interval:2h` | Run every N duration (30m, 2h, etc.) |
| `cron:EXPR` | `cron:0 8 * * 1-5` | Standard 5-field cron expression |
| `once:DATETIME` | `once:2024-01-15T08:00` | Run once at specified time |

### Actions

| Action | Description |
|--------|-------------|
| `list` | Show all configured reminders |
| `add` | Create a new reminder (requires: name, prompt, schedule) |
| `delete` | Remove a reminder by ID |
| `enable` | Enable a paused reminder |
| `disable` | Pause a reminder without deleting |
| `run` | Execute a reminder immediately |

### Examples

**Creating reminders (via chat):**

```
"Set a reminder to check my tasks every morning at 8am"
→ Creates: daily:08:00 reminder with prompt to list pending tasks

"Remind me every 2 hours to take a break"
→ Creates: interval:2h reminder

"Every weekday at 9am, summarize my calendar"
→ Creates: cron:0 9 * * 1-5 reminder
```

**How it works:**

1. The scheduler runs in the background, checking every minute for due reminders
2. When a reminder is due, its prompt is sent to the LLM with full tool access
3. The LLM executes the prompt (can use plugins, check tasks, etc.)
4. The response is sent to the original chat
5. The next run time is calculated based on the schedule type

Reminders are stored in SQLite and persist across restarts.

## Makefile Targets

| Target      | Description                    |
|-------------|--------------------------------|
| `build`     | Compile binary to `bin/tron`   |
| `run`       | Build and run                  |
| `run-debug` | Build and run with debug flag  |
| `clean`     | Remove build artifacts         |
| `test`      | Run tests                      |
| `vet`       | Run go vet                     |
| `fmt`       | Format code                    |
| `all`       | fmt, vet, test, build          |

## Usage

Once running, the bot:
- Responds to direct messages from the configured operator
- Responds to group messages prefixed with the trigger keyword (default: `T`)
- Maintains conversation context per chat
- Sends a daily summary at the configured hour
