# Setup Guide: Gemini CLI Server

This guide provides step-by-step instructions to get the **Gemini CLI Server** (with Telegram and WhatsApp support) running in your local environment.

## Prerequisites

- **Node.js** (>=20.0.0)
- **Go** (>=1.25.0) for building the bot binaries.
- **Gemini CLI** (installed and configured with your Google API Key).
- **NGROK** (optional, but recommended for remote access via webhooks).

---

## 1. Initial Project Setup

1.  **Clone the Repository:**
    ```bash
    git clone https://github.com/mpursley/gemini-cli-server.git
    cd gemini-cli-server
    ```

2.  **Install Node.js Dependencies:**
    ```bash
    npm install
    ```

3.  **Configure `.gemini` Directory:**
    For the Gemini CLI to access custom commands and scripts used by the listener:
    - Copy the `commands/` folder into your `~/.gemini/` directory.
    - Copy the `scripts/` folder into your `~/.gemini/` directory.

---

## 2. Platform-Specific Configuration

### Telegram Bot Setup
1.  **Create your Bot:** Message [@BotFather](https://t.me/botfather) on Telegram to create a new bot and get your **API Token**.
2.  **Environment Variables:** Create a file at `telegram_bot/.env`:
    ```bash
    TELEGRAM_BOT_TOKEN=your_token_here
    GEMINI_ENDPOINT=http://127.0.0.1:8765/event
    TARGET_CHAT_ID=  # optional: your chat ID to restrict access
    ```

### WhatsApp Bot Setup
1.  **Environment Variables:** Create a file at `whatsapp_bot/.env`:
    ```bash
    GEMINI_ENDPOINT=http://127.0.0.1:8765/event
    TARGET_JID=  # optional: e.g. 1234567890@s.whatsapp.net to restrict access
    ```
2.  **Initial Login:** Run the bot once manually to scan the QR code and link your account:
    ```bash
    cd whatsapp_bot
    go run main.go
    ```
    After linking, your session will be saved to `whatsapp_bot.db`.

---

## 3. Starting the Services

You can manage all services using the provided shell scripts in the `scripts/` directory.

### Start Everything (Listener + Telegram)
```bash
./scripts/manage_telegram.sh start
```

### Start WhatsApp
```bash
./scripts/manage_whatsapp.sh start
```

### Check Service Status
```bash
./scripts/manage_telegram.sh status
./scripts/manage_whatsapp.sh status
```

---

## 4. Remote Access (Optional)

If you're using NGROK to expose port 8765, you can test your listener by sending a message to the NGROK URL:
```bash
curl -X POST https://your-ngrok.ngrok-free.app/event \
     -H "Content-Type: application/json" \
     -d '{"source":"test","message":"Hello from cURL!"}'
```
This ensures your `listen.js` backend is correctly processing events and interacting with the Gemini CLI.
