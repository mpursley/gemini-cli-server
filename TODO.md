# TODO - gemini-cli-server

## Status Key

- `[ ]` TODO
- `[I]` In progress
- `[C]` Complete
- `[X]` Deployed to Prod/Active version

## Sessions & Persistence

- [X] **Implement `/save` command:** Enable the ability to save the current session with a custom name.
  - [X] Note: This requires updates to the `gemini-cli` repository to support a native save/rename function via the API/CLI Some changes were added to support this feature.. we need to test it out

- [X] **UI Consistency:** Add a prefix/header to the `Reply:` message in Telegram/WhatsApp (similar to the `💭 Thinking:` indicator) to clearly distinguish bot responses.

## Logging & Auditing

- [X] **Per-Session Logging:** Add a (Default) option to write all session text into individual log files.
  - _Path:_ `logs/sessions/<session_id>.txt`
  - _Format:_ `<date-time> : <message type> : <message text>`
  - _Example:_ `2026-03-28: Thought: I am thinking about how to find a solution...`

- [X] **Implement `/run` command:** Added an option to run commands on the cli/bash without sending them to gemini-cli. e.g. "/run ls -la". The output is sent in a code block.

## Future Ideas

- [ ] (Add more ideas here...)
