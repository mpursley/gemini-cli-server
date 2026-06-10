# Architecture Overview: Gemini CLI Server

The **Gemini CLI Server** follows a decoupled architecture, separating messaging platform logic from core AI and command processing.

## Components

The system consists of three primary layers:

### 1. Webhook Listener (`listen.js`)
Located in the `scripts/` directory, this Node.js script is the core backend. It:
- Listens for incoming POST requests on port `8765` (`/event` endpoint).
- Interfaces directly with the **Gemini CLI**.
- Manages user session state using the `--resume` flag.
- Handles MCP server interactions and shell command execution.
- Optionally logs all activity to the `logs/sessions/` directory.

### 2. Platform Bots (Go)
Each messaging platform has its own dedicated application written in Go:
- **Telegram Bot (`telegram_bot/main.go`):** Connects to the Telegram Bot API and dynamically registers commands from `.toml` files.
- **WhatsApp Bot (`whatsapp_bot/main.go`):** Uses the `whatsmeow` library to interface with WhatsApp's multi-device protocol.

These bots act as lightweight proxies: they receive user input, send it to the `listen.js` backend, and return the response to the user.

### 3. Gemini CLI & MCP
The **Gemini CLI** (the base tool) is invoked by `listen.js`. If the CLI is configured with **MCP (Model Context Protocol)** servers, those tools are automatically available to the mobile bots.

## Data Flow

The following diagram illustrates how a message travels through the system:

```text
User Message (TG/WA) 
       |
       v
Messaging Bot (Go) 
       |
       v  [HTTP POST to port 8765]
`listen.js` (Node.js) 
       |
       v  [Child Process Execution]
  Gemini CLI
       |
       v  [Command Output/AI Response]
`listen.js`
       |
       v  [HTTP Response]
Messaging Bot
       |
       v
   User Reply
```

## Session Management
- **Persistence:** Sessions are managed by the Gemini CLI itself. `listen.js` tracks which session ID is active for which user.
- **Resume Capability:** All interactions use the `--resume <session_id>` flag, ensuring context is never lost.
- **Shared Desktop/Mobile:** Because sessions are stored locally by the Gemini CLI, you can start a session on your desktop (`gemini --new`) and continue it on your phone by finding the session ID (`/sessions`) and attaching to it (`/attach <id>`).
