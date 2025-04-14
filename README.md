# mcp-proxy

A thin MCP proxy. It allows clients to connect MCP servers via HTTP without streaming transport.
It enables to put it on a simple infrastructure.

# How to install

```sh
go install github.com/ubie-oss/mcp-proxy
```

# How to use

## Prepare your mcp config file

Create a config file. The format is similar to Claude's. `$VARNAME` will be replaced with environment variables. It supports both `json` and `yaml`.

```json
{
  "mcpServers": {
    "server1": {
      "command": "npx",
      "args": ["module1"],
      "env": {
        "VAR1": "$SERVER1_VAR1",
        "VAR2": "$SERVER1_VAR2"
      }
    },
    "server2": {
      "command": "npx",
      "args": ["module2"],
      "env": {
        "VAR1": "$SERVER2_VAR1",
        "VAR2": "$SERVER2_VAR2"
      },
      "_extensions": {
        "tools": {
          "allow": [
            "tool1",
            "tool2"
          ],
          "deny": [
            "tool11",
            "tool12"
          ]
        }
      }
    }
  }
} 
```

```yaml
mcpServers:
  server1:
    command: "npx"
    args:
      - "module1"
    env:
      VAR1: $SERVER1_VAR1
      VAR2: $SERVER1_VAR2
  server2:
    command: "npx"
    args:
      - "module2"
    env:
      VAR1: $SERVER2_VAR1
      VAR2: $SERVER2_VAR2
    _extensions:
      tools:
        allow:
          - tool1
          - tool2
        deny:
          - tool11
          - tool12
```

### Tool filtering with allow/deny lists

You can control which tools are available to each MCP server using the `_extensions.tools` configuration:

- `allow`: Only tools listed here will be allowed. If this list is empty or not specified, all tools are allowed (unless denied).
- `deny`: Tools listed here will be blocked, even if they are in the allow list.

When both `allow` and `deny` lists are specified, the deny list takes precedence. That is, tools in the deny list will be blocked even if they also appear in the allow list.

## Run mcp-proxy with the config

```sh
go run . -config config.json -port 9090 
```
