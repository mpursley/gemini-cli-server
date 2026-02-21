# GeminiCLI Multi-Platform Bot (Telegram & WhatsApp)

This project provides Telegram and WhatsApp bot integrations for the Gemini CLI, enabling powerful AI-driven automation and interactive responses directly within your favorite messaging apps. The core idea is to bridge the gap between conversational interfaces and robust command-line automation, allowing Gemini to participate in conversations, respond to messages, and automate tasks in a helpful and informative way—allowing you to move away from your machine while continuing your "vibe coding" sessions on the go.

This project was inspired by the original Slack bot concept from [John Capobianco](https://github.com/automateyournetwork/GeminiCLI_Slash_Listen).

## Installation

To get started, you'll need to clone this repository and set up the necessary components.

### Cloning the Repository

First, clone the repository to your local machine:

```bash
git clone https://github.com/bravian1/gemini_cli_server.git
```

### Project Setup

After cloning, copy both the `commands` and `scripts` folders into your `.gemini` directory. This ensures the Gemini CLI can access the necessary configurations and scripts for the Telegram bot integration.

## 🚀 New: Session History Support

The Gemini CLI Server now automatically maintains conversation history per user across both platforms. This means the bot will "remember" previous parts of your conversation, allowing for more natural and contextual "vibe coding" on the go.

- **Automatic Persistence:** Each user has their own dedicated session managed by the Gemini CLI.
- **Native Resumption:** The bot uses the Gemini CLI's `--resume` flag internally to maintain context.
- **Multi-Platform Support:** Both Telegram and WhatsApp bots support session list, attachment, and creation.
- **Local Management:** You can still use the standard Gemini CLI commands on your machine to list or resume these sessions:
    - `gemini --list-sessions`
    - `gemini --resume <session_id>`

## Core Components and Interaction

This integration relies on three main components:

1.  **`listen.js` (Gemini CLI Webhook Listener):** Located in the `scripts/` directory, this Node.js script acts as an HTTP server on port 8765 that listens for incoming messages and events. It serves as the shared backend for both messaging bots.
2.  **`main.go` (Telegram Bot):** Located in the `telegram_bot/` directory, this Go application interacts with the Telegram Bot API.
3.  **`main.go` (WhatsApp Bot):** Located in the `whatsapp_bot/` directory, this Go application uses the `whatsmeow` library to interact with WhatsApp Multi-Device.

**Interaction Flow:**

User Message (TG/WA) -> Messaging Bot (Go) -> `listen.js` (Node.js) -> Gemini CLI -> `listen.js` -> Messaging Bot -> User Reply

## Telegram Bot Setup

To get your Telegram bot up and running:

1.  **Create a bot with [@BotFather](https://t.me/botfather) on Telegram.** Follow the instructions to create a new bot and obtain your unique bot token.
2.  **Set up environment variables in `telegram_bot/.env`:**

    ```bash
    TELEGRAM_BOT_TOKEN=your_bot_token_here
    GEMINI_ENDPOINT=http://127.0.0.1:8765/event
    TARGET_CHAT_ID=                            # optional: specific chat ID for restricted access
    ```

## WhatsApp Bot Setup

To get your WhatsApp bot up and running:

1.  **Ensure you have Go 1.25.0+ installed.**
2.  **Set up environment variables in `whatsapp_bot/.env`:**

    ```bash
    GEMINI_ENDPOINT=http://127.0.0.1:8765/event
    TARGET_JID=                                # optional: e.g. 1234567890@s.whatsapp.net to restrict access
    ```
3.  **Perform Initial Login (Scan QR Code):**
    Run the bot manually once to scan the QR code and link your device:
    ```bash
    cd whatsapp_bot
    go run main.go
    ```
    After scanning, the session will be saved to `whatsapp_bot.db`.

## Service Management

You can manage all services (Listener, Telegram, and WhatsApp) using the provided shell scripts in the `scripts/` directory.

### Managing Everything (Telegram + Listener)
```bash
./scripts/manage_telegram.sh start
./scripts/manage_telegram.sh stop
./scripts/manage_telegram.sh status
./scripts/manage_telegram.sh logs
```

### Managing WhatsApp
```bash
./scripts/manage_whatsapp.sh start
./scripts/manage_whatsapp.sh stop
./scripts/manage_whatsapp.sh status
./scripts/manage_whatsapp.sh logs
```

## Usage: Messaging Bot Commands

Both bots support common commands to manage AI sessions:

*   `/sessions` - Lists recent sessions.
*   `/attach <session_id>` - Resumes a specific session.
*   `/new` - Starts a fresh session.
*   `/status` - Shows the current session ID and bot status.

Additionally, the Telegram bot dynamically registers commands based on `.toml` files found in the `commands/listen` directory.

## Testing your listener externally
If you NGROK out your local 8765 port, you can test your listener by sending a message to the NGROK URL with the following command:
```bash
curl -X POST https://your-ngrok.ngrok-free.app/event -H "Content-Type: application/json" -d '{"source":"test","message":"This is a test message from cURL to Gemini CLI. If you are really Gemini CLI please respond with a message that, yes, you are really Gemini CLI and a pleasant haiku for the tester."}'
```

## MCP Integration
If your Gemini CLI is integrated with MCP servers they are fully accessible via the Telegram bot. Meaning Gemini CLI will invoke those MCP servers when a message is received if they will help respond to the message.
