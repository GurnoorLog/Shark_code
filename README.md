# SHARKCODE 🦈

Yoo gng. This is a shark-themed coding agent that lives in your terminal. Give it a task in plain English and it plans the work, runs the commands, edits files, and checks its own output until the job is done. Built with Go and Bubble Tea.

## How it works

Each turn is a loop: your prompt goes to an LLM, the model calls tools, the tools run on your machine, results go back to the model, and it keeps going until it has a final answer (up to 25 steps per turn).

Tools it can call:

- `bash`: run shell commands (runs cmd.exe on Windows, see below)
- `read_file`, `write_file`, `edit_file`, `delete_file`
- `list_dir`, `glob`, `grep` (substring match, walks the tree, skips `.git`)
- `web_search` (DuckDuckGo), `web_fetch` (reads pages as text), `download_file` (saves binaries like images or zips)

Nothing touches your machine without asking. Every tool call pops a permission box: y/enter allows, n/esc denies.

There is also an accuracy gate. After a turn that used tools, the model gets one extra pass to review its work and fix mistakes. It's on by default, toggle with `/verify`.

## Plan and build modes

Press Tab to switch. In plan mode the mutating tools (`bash`, `write_file`, `edit_file`, `delete_file`, `download_file`) are hidden and blocked, so the model can only read around and produce a plan without changing anything. Tab again to build for real.

## Models

Five providers out of the box: OpenAI, Anthropic (Claude), Gemini, Fireworks, and a local llama-server. Hit `/model` for the interactive picker, save keys with `/key`.

| provider | base url | default model |
|----------|----------|---------------|
| openai | https://api.openai.com/v1 | gpt-4o |
| anthropic | https://api.anthropic.com | claude-sonnet-4-20250514 |
| gemini | https://generativelanguage.googleapis.com/v1beta | gemini-2.0-flash |
| fireworks | https://api.fireworks.ai/inference/v1 | accounts/fireworks/models/minimax-m3 |
| local | http://127.0.0.1:8080/v1 | gemma-2-2b-it |

Local models get auto-discovered from your llama-server.

## The TUI

Underwater theme, animated background, bubbles rising behind everything. Replies stream in token by token as they generate. There's an opencode-style model picker, slash-command autocomplete while you type, prompt history with the arrow keys, scrollback with PgUp/PgDn or the mouse wheel, and esc esc to cancel a running task. The status bar tracks tokens spent, cost, and how much of the context window you've used.

Copying a response takes one keypress: hit Ctrl+Y or type /copy to put the latest answer on your clipboard, and /copy 2, /copy 3, and so on to grab older ones. The chat shows a 🦈 msg copied ✓ line when it works. You can also select text the usual terminal way: hold Shift, drag across the words you want, then copy with Ctrl+Shift+C or right-click.

Commands:

```
/model [provider] [model]   switch models (no args opens the picker)
/key <provider> <KEY>       save an API key
/verify                     toggle the accuracy gate
/plan / /build              switch modes (Tab does this too)
/providers                  list providers and models
/copy [n]                   copy answer to the clipboard (Ctrl+Y too)
/clear                      fresh conversation
/help                       all commands
/exit                       quit
```

## Works on all systems

SHARKCODE runs on Windows, macOS, and Linux. Before your first message it figures out what machine it's on: operating system and architecture, which shell the `bash` tool will actually drive, your real home directory and username, plus an OS version probe (`ver` on Windows, `sw_vers` on macOS, `/etc/os-release` or `uname` on Linux). All of that gets injected into its system prompt, so it acts on facts instead of guessing paths and hitting "access denied".

On Windows the `bash` tool invokes cmd.exe, and common Unix idioms get translated so the model doesn't trip over them: `mkdir -p` becomes `mkdir`, `rm -rf` becomes `rmdir /s /q`, `touch` becomes `type nul >`, and `~` expands to your real home directory.

On macOS and Linux it drives `/bin/sh`, so any distro works: Ubuntu, Debian, Fedora, Arch, Mint, Pop!_OS, openSUSE, Alpine, and the rest. It identifies your specific distro from `/etc/os-release` and knows to reach for apt/dnf/pacman only when something genuinely needs installing.

When things go wrong, it tells you instead of dying quietly: provider hiccups get retried automatically, tool errors go back to the model so it can change approach, and if it hits its step budget it pauses and asks you to say "continue".

## Build and run

```bash
go build -o sharkcode.exe .
./sharkcode.exe
```

API keys and the active model persist in `~/.sharkcode/config.json`.

## Example session

```
> /key gemini AIzaxxxxxxxx
> /model gemini
> create a python script that prints fibonacci, then run it
```

The agent writes the file, asks before running it, and shows you the output.
