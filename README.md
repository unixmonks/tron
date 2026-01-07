# Tron

> **⚠️ Experimental Project** - This project is under active development and changing rapidly. APIs, configuration, and features may change without notice.
>
> **🔓 Security Warning** - This project is highly insecure. It executes arbitrary plugins, has no authentication beyond Signal's, and should only be run in trusted environments. Do not expose to untrusted users.

A Signal messaging bot with LLM integration and plugin support.

## Quick Examples

Once running, chat with the bot naturally. It uses plugins automatically based on your requests:

```
You: "What tasks do I have pending?"
Bot: [invokes task plugin]
    You have 3 pending tasks:
    1. Call the plumber (due: tomorrow)
    2. Review budget spreadsheet (due: Friday)
    3. Book dentist appointment

You: "Add a task to pick up groceries"
Bot: [invokes task plugin]
    Added: "Pick up groceries"

You: "What's using all my CPU?"
Bot: [invokes ps plugin]
    Top processes by CPU:
    - firefox (12.3%)
    - code (8.1%)
    - slack (4.2%)

You: "Mark task 1 as done"
Bot: [invokes task plugin]
    Completed: "Call the plumber"
```

In group chats, prefix messages with the trigger keyword (default: `T`):

```
You: "T what tasks are due today?"
Bot: [responds to the group]
```

## Prerequisites

- Go 1.25.3+
- [signal-cli](https://github.com/AsamK/signal-cli) running in JSON-RPC daemon mode
- An LLM API endpoint (OpenAI-compatible, e.g., DeepInfra, OpenAI, local Ollama)
- SQLite3 (for conversation memory)
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
