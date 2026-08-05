# SHARKCODE 🦈

A shark-themed terminal coding agent built with Go + Bubble Tea. It runs commands, edits files, installs dependencies, and does everything a coding agent should — right in your terminal.

## Features

- **Multi-provider AI** — OpenAI, Claude (Anthropic), Gemini, Fireworks, and local models (llama-server)
- **Slash commands** — `/model`, `/key`, `/providers`, `/clear`, `/help`, `/exit`
- **Tool calling** — bash, read_file, write_file, list_dir, glob, grep
- **Shark TUI** — themed interface with a "sharking" spinner
- **Persistent config** — API keys and model choices saved to `~/.sharkcode/config.json`

## Build

```bash
go build -o sharkcode.exe .
```

## Run

```bash
./sharkcode.exe
```

## First-time setup

```bash
# set API keys
/key openai sk-xxxxxxxx
/key anthropic sk-ant-xxxxxxxx
/key gemini AIzaxxxxxxxx
/key fireworks fw_xxxxxxxx

# switch model
/model openai gpt-4o
/model anthropic claude-sonnet-4-20250514
/model gemini gemini-2.0-flash
/model fireworks accounts/fireworks/models/minimax-m3

# use a local llama-server (running on 127.0.0.1:8080)
/model local gemma-2-2b-it
```

## Example session

```
> /key fireworks fw_xxxxxxxx
> /model fireworks accounts/fireworks/models/minimax-m3
> create a python script that prints fibonacci, then run it
```

The agent will write the file and run it via its tools.

## Providers

| Provider | Base URL | Model |
|----------|----------|-------|
| openai | https://api.openai.com/v1 | gpt-4o |
| anthropic | https://api.anthropic.com | claude-sonnet-4-20250514 |
| gemini | https://generativelanguage.googleapis.com/v1beta | gemini-2.0-flash |
| fireworks | https://api.fireworks.ai/inference/v1 | accounts/fireworks/models/minimax-m3 |
| local | http://127.0.0.1:8080/v1 | gemma-2-2b-it |
