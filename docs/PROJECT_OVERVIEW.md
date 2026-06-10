# Project Overview: Gemini CLI Server

The **Gemini CLI Server** is a multi-platform messaging integration for the [Gemini CLI](https://github.com/google/gemini-cli). It allows users to interact with the Gemini AI model and execute CLI-driven automations directly from mobile messaging apps like **Telegram** and **WhatsApp**.

## Core Mission

The primary goal of this project is to enable "vibe coding" on the go. It bridges the gap between powerful local CLI tools and the convenience of mobile messaging, allowing developers to:
- Monitor long-running tasks.
- Initiate new coding or automation workflows from their phone.
- Maintain persistent AI session context across devices.
- Access local MCP (Model Context Protocol) servers remotely.

## Key Features

- **Multi-Platform Support:** Native bots for Telegram and WhatsApp.
- **Session Persistence:** Automatically maintains conversation history per user, allowing you to resume desktop sessions on mobile and vice versa.
- **Dynamic Commands:** Telegram commands are dynamically registered based on configuration files.
- **Local Tool Access:** Execute shell commands remotely via the `/run` command.
- **MCP Integration:** Full access to all Model Context Protocol servers configured in the local Gemini CLI environment.
- **Browser Agent Testing:** Integrated support for the Gemini CLI "Browser Agent," allowing for automated testing of web features and UI flows directly through the AI.
- **Session Logging:** Optional per-session logging for auditing and history tracking.

## Origins

This project was inspired by the Slack bot concept created by [John Capobianco](https://github.com/automateyournetwork/GeminiCLI_Slash_Listen).
