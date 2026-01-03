# Plugins

Tron supports two types of plugins: **external plugins** (shell scripts, Python scripts, etc.) and **internal tools** (Go-based tools compiled into the binary).

## External Plugins

External plugins are stored in the `plugins.d/` directory. Each plugin lives in its own subdirectory.

### Directory Structure

```
plugins.d/
├── task/
│   ├── definition.json
│   └── run
└── ps/
    ├── definition.json
    └── run
```

### Included Plugins

| Plugin | Description | Requirements |
|--------|-------------|--------------|
| `task` | Taskwarrior integration (list, add, complete, modify, delete tasks) | [Taskwarrior](https://taskwarrior.org/) |
| `ps` | List and filter running system processes | None |

### Using Plugins

Plugins are automatically loaded at startup. The LLM decides when to invoke a plugin based on the user's request and the plugin's description.

**Examples:**

```
User: "Show me my pending tasks"
Bot: [invokes task plugin with action: "list"]

User: "Add a task to call the plumber tomorrow"
Bot: [invokes task plugin with action: "add", description: "Call the plumber", due: "tomorrow"]

User: "What processes are using the most CPU?"
Bot: [invokes ps plugin with sort: "cpu", limit: 10]
```

## Creating a Plugin

### 1. Create the Plugin Directory

```bash
mkdir plugins.d/myplugin
```

### 2. Create definition.json

The definition file describes your plugin to the LLM:

```json
{
  "name": "myplugin",
  "description": "Brief description of what this plugin does. Be specific so the LLM knows when to use it.",
  "enabled": true,
  "timeout": 30,
  "parameters": {
    "type": "object",
    "properties": {
      "action": {
        "type": "string",
        "enum": ["list", "add", "delete"],
        "description": "The action to perform"
      },
      "query": {
        "type": "string",
        "description": "Search query or item name"
      },
      "count": {
        "type": "integer",
        "description": "Number of results to return"
      }
    },
    "required": ["action"]
  }
}
```

**Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Unique plugin identifier |
| `description` | string | yes | Description shown to the LLM |
| `enabled` | boolean | no | Set to `false` to disable (default: `true`) |
| `timeout` | integer | no | Execution timeout in seconds (default: 30) |
| `parameters` | object | yes | JSON Schema describing accepted parameters |

### 3. Create the Executable

Create a file named `run` (or `run.sh`, `run.py`, `run.rb`, `main`) and make it executable:

```bash
chmod +x plugins.d/myplugin/run
```

**Input:** JSON is passed via **stdin** containing the parameters.

**Output:** Write JSON to **stdout** for success.

**Errors:** Write error messages to **stderr** and exit with non-zero status.

### Example: Bash Plugin

```bash
#!/bin/bash
set -e

input=$(cat)
action=$(echo "$input" | jq -r '.action // empty')
query=$(echo "$input" | jq -r '.query // empty')

case "$action" in
    list)
        # Return JSON array
        echo '["item1", "item2", "item3"]'
        ;;
    search)
        if [[ -z "$query" ]]; then
            echo '{"error": "query is required for search"}' >&2
            exit 1
        fi
        echo "{\"results\": [], \"query\": \"$query\"}"
        ;;
    *)
        echo "{\"error\": \"unknown action: $action\"}" >&2
        exit 1
        ;;
esac
```

### Example: Python Plugin

```python
#!/usr/bin/env python3
import json
import sys

def main():
    args = json.load(sys.stdin)
    action = args.get('action')

    if action == 'list':
        result = {'items': ['a', 'b', 'c']}
        print(json.dumps(result))
    elif action == 'add':
        name = args.get('name')
        if not name:
            print(json.dumps({'error': 'name is required'}), file=sys.stderr)
            sys.exit(1)
        print(json.dumps({'status': 'added', 'name': name}))
    else:
        print(json.dumps({'error': f'unknown action: {action}'}), file=sys.stderr)
        sys.exit(1)

if __name__ == '__main__':
    main()
```

### Parameter Schema

The `parameters` field uses [JSON Schema](https://json-schema.org/) to define accepted inputs:

```json
{
  "parameters": {
    "type": "object",
    "properties": {
      "action": {
        "type": "string",
        "enum": ["get", "set", "delete"],
        "description": "Operation to perform"
      },
      "key": {
        "type": "string",
        "description": "The key name"
      },
      "value": {
        "type": "string",
        "description": "Value to set (for set action)"
      },
      "tags": {
        "type": "array",
        "items": {"type": "string"},
        "description": "Optional tags"
      },
      "force": {
        "type": "boolean",
        "description": "Force operation without confirmation"
      }
    },
    "required": ["action", "key"]
  }
}
```

### Testing Your Plugin

Test manually by piping JSON to your executable:

```bash
echo '{"action": "list"}' | ./plugins.d/myplugin/run

echo '{"action": "add", "name": "test item"}' | ./plugins.d/myplugin/run
```

Enable debug logging to see plugin invocations:

```bash
make run-debug
```

### Best Practices

1. **Keep output concise** - The LLM processes the output, so avoid excessive data
2. **Return structured JSON** - Makes it easier for the LLM to interpret results
3. **Use descriptive error messages** - Help the LLM understand what went wrong
4. **Handle edge cases** - Check for missing required parameters
5. **Set appropriate timeouts** - Long-running operations should increase the timeout

## Internal Tools

Internal tools are Go-based plugins compiled into the Tron binary. They have direct access to the database and other internal systems.

### Built-in Internal Tools

| Tool | Description |
|------|-------------|
| `reminder` | Schedule and manage reminders (see [Reminders](#reminders) in README) |

### Creating an Internal Tool

Internal tools implement the `InternalTool` interface:

```go
package mytools

import "tron"

type MyTool struct{}

func (t *MyTool) Definition() tron.Tool {
    return tron.Tool{
        Type: "function",
        Function: tron.ToolFunction{
            Name:        "mytool",
            Description: "Description of what this tool does",
            Parameters: map[string]interface{}{
                "type": "object",
                "properties": map[string]interface{}{
                    "action": map[string]interface{}{
                        "type":        "string",
                        "description": "The action to perform",
                    },
                },
                "required": []string{"action"},
            },
        },
    }
}

func (t *MyTool) Execute(argsJSON string) (string, error) {
    // Parse argsJSON and perform the action
    return "result", nil
}
```

Register the tool with the plugin manager in `main.go`:

```go
pluginMgr.RegisterTool("mytool", &mytools.MyTool{})
```

For tools that need conversation context (e.g., which chat the message came from), implement `ContextAwareTool`:

```go
type ContextAwareTool interface {
    InternalTool
    SetContext(chatID string)
}
```

## Plugin Configuration

### Disabling a Plugin

Set `"enabled": false` in the plugin's `definition.json`:

```json
{
  "name": "task",
  "enabled": false,
  ...
}
```

### Changing Plugin Directory

Set the plugin directory in config:

```yaml
plugin_dir: "my-plugins"
```

Or via environment variable:

```bash
export PLUGIN_DIR="my-plugins"
```
