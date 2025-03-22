# mcp-proxy

A thin MCP proxy. It allows clients to connect MCP servers via HTTP without SSE/streaming custom transport.
It enables to put it on a simple infrastructure.

# How to install

```sh
go get github.com/ubie-oss/mcp-proxy
```

# How to use

## Prepare your mcp config file

Create a config file. The format is similar to Claude's. It supports both `json` and `yaml`.

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
      }
    }
  }
} 
```

## Run mcp-proxy with the config

```sh
go run . -config config.json -port 9090 
```
