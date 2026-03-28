# TODO - gemini-cli-server

## Status Key
[ ] : Todo
[I] : Inprogress, need to work on this more
[T] : Implimented, but needed testing
[C] : Tested and working, but needs to be commited to a branch in the repo
[P] : In a PR, but not merged into main yet
[D] : Done, tast/subtask on complete

## Sessions & Persistence
- [C] **Implement `/save` command:** Enable the ability to save the current session with a custom name.
    - [C] Note: This requires updates to the `gemini-cli` repository to support a native save/rename function via the API/CLI Some changes were added to support this feature.. we need to test it out

- [ ] **UI Consistency:** Add a prefix/header to the `Reply:` message in Telegram/WhatsApp (similar to the `💭 Thinking:` indicator) to clearly distinguish bot responses.

## Logging & Auditing
- [ ] **Per-Session Logging:** Add a (Default) option to write all session text into individual log files.
    - *Path:* `logs/sessions/<session_id>.txt`
    - *Format:* `<date-time> : <message type> : <message text>`
    - *Example:* `2026-03-28: Thought: I am thinking about how to find a solution...`

## Future Ideas
- [ ] (Add more ideas here...)
