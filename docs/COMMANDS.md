# Command Reference: Gemini CLI Server

Both the **Telegram** and **WhatsApp** bots support a set of core commands to manage your AI sessions and interact with your local environment.

## Core Session Commands

- **`/new`**:
  Starts a completely fresh session. Any previous context is ignored, and a new session ID is assigned.
- **`/sessions [filter]`**:
  Lists the most recent sessions stored by the Gemini CLI.
  - _Optional:_ Provide a filter string to search for sessions by their description (e.g., `/sessions project-x`).
- **`/attach <session_id>`**:
  Resumes a specific existing session. All subsequent messages will be sent within the context of this session.
- **`/status`**:
  Displays the ID of the current active session and the general health of the bot.
- **`/save [name]`**:
  Saves the current active session. If a name is provided, it attempts to rename the session for easier identification later.
- **`/delete <session_id>`**:
  Removes a session and its associated data from the Gemini CLI's local store.

## System Commands

- **`/run <command>`**:
  Executes a shell command directly on the host machine where `listen.js` is running.
  - _Output:_ The command's standard output (stdout) is returned to the chat within a code block.
  - _Caution:_ Use this command with care, as it provides remote shell access to the host.

## Dynamic Commands (Telegram Only)

The Telegram bot can dynamically register additional commands based on `.toml` configuration files located in the `commands/listen` directory. These are typically used for specific automation or AI behaviors that require custom prompting.

## Interaction Patterns

- **Direct Messages:** Sending any text that doesn't start with a `/` will be treated as a message to the AI in the current active session.
- **"Thinking" Indicator:** While the AI is processing your request (especially for complex tasks or MCP tool execution), the bot will show a `💭 Thinking...` status message.
- **AI Replies:** Responses from the AI are prefixed for clarity, helping you distinguish between system messages and AI-generated content.
