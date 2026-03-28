package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
)

type GeminiPayload struct {
	Source    string `json:"source"`
	Message   string `json:"message"`
	SessionID string `json:"sessionId,omitempty"`
	ImageData string `json:"imageData,omitempty"`
	MimeType  string `json:"mimeType,omitempty"`
	ApiKey    string `json:"apiKey,omitempty"`
}

type StreamChunk struct {
	Type      string `json:"type"`
	Role      string `json:"role,omitempty"`
	Subject   string `json:"subject,omitempty"`
	Content   string `json:"content,omitempty"`
	SessionId string `json:"session_id,omitempty"`
	Model     string `json:"model,omitempty"`
	ToolName  string `json:"tool_name,omitempty"`
	Status    string `json:"status,omitempty"`
	Error     string `json:"error,omitempty"`
}

type GeminiResponse struct {
	Reply     string `json:"reply"`
	SessionID string `json:"sessionId"`
	Model     string `json:"model"`
}

type CommandConfig struct {
	Description string `toml:"description"`
}

type UserState struct {
	State     string
	SessionID string
}

var (
	bot          *tgbotapi.BotAPI
	geminiURL    string
	geminiAPIKey string
	targetChatID int64
	userStates   = make(map[int64]*UserState)
	envFilePath  = ".env"
)

const (
	maxVoiceDurationSeconds = 300              // 5 minutes
	maxVoiceFileSizeBytes   = 50 * 1024 * 1024 // 50MB
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found, will create one if needed")
	}

	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN environment variable is required")
	}

	geminiURL = os.Getenv("GEMINI_ENDPOINT")
	if geminiURL == "" {
		geminiURL = "http://127.0.0.1:8765/event"
	} else if !strings.HasPrefix(geminiURL, "http://") && !strings.HasPrefix(geminiURL, "https://") {
		geminiURL = "http://" + geminiURL
	}
	if strings.HasPrefix(geminiURL, "https://") {
		geminiURL = strings.Replace(geminiURL, "https://", "http://", 1)
	}

	geminiAPIKey = os.Getenv("GEMINI_API_KEY")

	if chatID := os.Getenv("TARGET_CHAT_ID"); chatID != "" {
		if id, err := strconv.ParseInt(chatID, 10, 64); err == nil {
			targetChatID = id
		}
	}

	var err error
	bot, err = tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Authorized on account %s", bot.Self.UserName)
	log.Printf("Gemini endpoint: %s", geminiURL)
	log.Printf("Target chat ID: %d", targetChatID)

	// Register commands
	commands := []tgbotapi.BotCommand{
		{Command: "sessions", Description: "List recent sessions"},
		{Command: "attach", Description: "Attach to a session (e.g. /attach ID)"},
		{Command: "save", Description: "Save current session (e.g. /save name)"},
		{Command: "new", Description: "Start a new session"},
		{Command: "status", Description: "Show current session status"},
		{Command: "restart", Description: "Restart the Telegram bot and server"},
	}
	config := tgbotapi.NewSetMyCommands(commands...)
	if _, err := bot.Request(config); err != nil {
		log.Printf("Error setting bot commands: %v", err)
	}

	if geminiAPIKey != "" {
		log.Printf("Gemini API key loaded from environment")
	} else {
		log.Printf("No Gemini API key found - voice transcription will require user input")
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil {
			go handleMessage(update.Message)
		}
	}
}

type uiUpdate struct {
	thought string
	text    string
	timer   string
	bump    bool
}

func startUIUpdater(ctx context.Context, chatID int64, initialMsgID int) (chan uiUpdate, chan bool) {
	editChan := make(chan uiUpdate, 100)
	doneChan := make(chan bool)

	go typingIndicator(ctx, chatID, editChan)

	go func() {
		var lastEdit time.Time
		var lastThought string
		var lastText string
		var lastTimer string
		thoughtMsgID := initialMsgID
		var textMsgID int

		timer := time.NewTimer(time.Hour)
		timer.Stop()

		for {
			select {
			case update := <-editChan:
				if update.thought != "" {
					lastThought = update.thought
				}
				if update.text != "" {
					lastText = update.text
				}
				if update.timer != "" {
					lastTimer = update.timer
				}

				if update.bump {
					msgInfo := "💭 *Thinking:*\n" + lastThought
					if lastTimer != "" {
						msgInfo += "\n\n_" + lastTimer + "_"
					}
					newMsg := tgbotapi.NewMessage(chatID, msgInfo)
					newMsg.ParseMode = "Markdown"
					sent, err := bot.Send(newMsg)
					if err == nil {
						deleteMsg := tgbotapi.NewDeleteMessage(chatID, thoughtMsgID)
						bot.Request(deleteMsg)
						thoughtMsgID = sent.MessageID
					}
				}

				if time.Since(lastEdit) > 2*time.Second {
					// Handle Thought edits
					if lastThought != "" || lastTimer != "" {
						msgInfo := "💭 *Thinking:*\n" + lastThought
						if lastTimer != "" {
							msgInfo += "\n\n_" + lastTimer + "_"
						}
						editMsg := tgbotapi.NewEditMessageText(chatID, thoughtMsgID, msgInfo)
						editMsg.ParseMode = "Markdown"
						bot.Send(editMsg)
					}

					// Handle Text response edits
					if lastText != "" {
						if textMsgID == 0 {
							// First time we get text, send a new message
							msg := tgbotapi.NewMessage(chatID, lastText)
							msg.ParseMode = "Markdown"
							sent, err := bot.Send(msg)
							if err == nil {
								textMsgID = sent.MessageID
							}
						} else {
							// Subsequent text updates, edit the second message
							editMsg := tgbotapi.NewEditMessageText(chatID, textMsgID, lastText)
							editMsg.ParseMode = "Markdown"
							bot.Send(editMsg)
						}
					}
					lastEdit = time.Now()
				} else {
					timer.Reset(2 * time.Second)
				}
			case <-timer.C:
				// Push final update
				if lastThought != "" || lastTimer != "" {
					msgInfo := "💭 *Thinking:*\n" + lastThought
					if lastTimer != "" {
						msgInfo += "\n\n_" + lastTimer + "_"
					}
					editMsg := tgbotapi.NewEditMessageText(chatID, thoughtMsgID, msgInfo)
					editMsg.ParseMode = "Markdown"
					bot.Send(editMsg)
				}
				if lastText != "" && textMsgID != 0 {
					editMsg := tgbotapi.NewEditMessageText(chatID, textMsgID, lastText)
					editMsg.ParseMode = "Markdown"
					bot.Send(editMsg)
				}
				lastEdit = time.Now()
			case <-doneChan:
				return
			}
		}
	}()

	return editChan, doneChan
}

func typingIndicator(ctx context.Context, chatID int64, editChan chan uiUpdate) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	dots := "..."
	seconds := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			seconds += 5
			dots += ".."
			if len(dots) > 10 {
				dots = "..."
			}

			timerText := fmt.Sprintf("Thinking (%d seconds) %s", seconds, dots)
			bump := false
			if seconds%60 == 0 {
				timerText = fmt.Sprintf("Still thinking (%d seconds) %s", seconds, dots)
				bump = true
			}

			editChan <- uiUpdate{timer: timerText, bump: bump}

			// Refresh the telegram "typing" status
			bot.Send(tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping))
		}
	}
}

func getUserState(userID int64) *UserState {
	if state, exists := userStates[userID]; exists {
		return state
	}
	userStates[userID] = &UserState{}
	return userStates[userID]
}

type Session struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Time        string `json:"time"`
}

type SessionsResponse struct {
	Ok       bool      `json:"ok"`
	Sessions []Session `json:"sessions"`
	Error    string    `json:"error,omitempty"`
}

func handleMessage(message *tgbotapi.Message) {
	if message.From.IsBot {
		return
	}

	if targetChatID != 0 && message.Chat.ID != targetChatID {
		return
	}

	// Handle user state for API key setup
	userState := getUserState(message.From.ID)
	if userState.State == "waiting_api_key" {
		handleAPIKeyInput(message)
		return
	}

	// Handle voice messages
	if message.Voice != nil {
		handleVoiceMessage(message)
		return
	}

	// Handle photo messages
	if message.Photo != nil && len(message.Photo) > 0 {
		handlePhotoMessage(message)
		return
	}

	text := strings.TrimSpace(message.Text)
	if text == "" {
		return
	}

	// Handle Commands
	if strings.HasPrefix(text, "/") {
		parts := strings.Fields(text)
		command := parts[0]

		switch command {
		case "/start":
			msg := tgbotapi.NewMessage(message.Chat.ID, "👋 Welcome! I'm your Gemini assistant. Send me a message or a voice note to get started.\n\nCommands:\n/sessions - List recent sessions\n/attach <id> - Attach to a session\n/save <name> - Save current session\n/new - Start a new session\n/restart - Restart the bot and server")
			bot.Send(msg)
			return
		case "/new":
			userState.SessionID = ""
			msg := tgbotapi.NewMessage(message.Chat.ID, "🆕 Started a new session.")
			bot.Send(msg)
			return
		case "/restart":
			msg := tgbotapi.NewMessage(message.Chat.ID, "🔄 Restarting gemini-cli-server...")
			bot.Send(msg)
			
			// Get the absolute path to the manage script
			repoDir := os.Getenv("HOME") + "/dev/gemini-cli-server"
			scriptPath := repoDir + "/scripts/manage_telegram.sh"
			
			// Run the restart command in the background
			go func() {
				// Wait a brief moment for the "Restarting..." message to be sent
				time.Sleep(1 * time.Second)
				
				// Execute the restart script
				// Note: Since this will kill the current process, we use a separate shell session
				// and don't care about the result here as the process will terminate.
				spawnRestart(scriptPath, repoDir)
			}()
			return
		case "/status":
			sessionID := "None"
			if userState.SessionID != "" {
				sessionID = fmt.Sprintf("`%s`", userState.SessionID)
			}
			msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("📊 *Bot Status*\n\n🔗 Session: %s\n🎤 Voice: Supported", sessionID))
			msg.ParseMode = "Markdown"
			bot.Send(msg)
			return
		case "/sessions":
			handleSessionsCommand(message)
			return
		case "/attach":
			if len(parts) < 2 {
				msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Please provide a session ID. Example: `/attach 8a3d000a...`")
				msg.ParseMode = "Markdown"
				bot.Send(msg)
				return
			}
			userState.SessionID = parts[1]
			msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("🔗 Attached to session: `%s`", userState.SessionID))
			msg.ParseMode = "Markdown"
			bot.Send(msg)
			return
		case "/save":
			if len(parts) < 2 {
				msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Please provide a name. Example: `/save my_session`")
				msg.ParseMode = "Markdown"
				bot.Send(msg)
				return
			}
			if userState.SessionID == "" {
				msg := tgbotapi.NewMessage(message.Chat.ID, "❌ You do not have an active session to save. Please start chatting first or use `/attach <id>` to resume an old session.")
				msg.ParseMode = "Markdown"
				bot.Send(msg)
				return
			}
			// Transform to the native CLI command and let it fall through to be sent to the backend
			text = "/resume save " + strings.Join(parts[1:], " ")
		default:
			// Let unknown commands fall through as regular text if needed, or return
			// But since we fall through on /save, we just let the switch finish.
		}
	}

	log.Printf("Processing message from %s (Current Session: %s): %s", message.From.UserName, userState.SessionID, text)

	var prompt string
	ctxText := ""
	if message.ReplyToMessage != nil {
		ctxText = fmt.Sprintf("Context: %s: %s\n\n",
			message.ReplyToMessage.From.FirstName,
			message.ReplyToMessage.Text)
	}
	prompt = fmt.Sprintf("%sYou are an assistant in a Telegram chat.\nAnswer this message:\n\n%s: %s",
		ctxText, message.From.FirstName, text)

	// Special case for administrative commands intended for the backend
	if strings.HasPrefix(text, "/resume save ") {
		prompt = text
	}

	// Send initial thinking message
	thinkingMsg := tgbotapi.NewMessage(message.Chat.ID, "Thinking...")
	if message.ReplyToMessage != nil {
		thinkingMsg.ReplyToMessageID = message.MessageID
	}
	sentMsg, err := bot.Send(thinkingMsg)
	if err != nil {
		log.Printf("Error sending thinking message: %v", err)
	}

	// Start typing indicator and UI updater
	ctx, cancelIndicator := context.WithCancel(context.Background())
	editChan, doneChan := startUIUpdater(ctx, message.Chat.ID, sentMsg.MessageID)

	var finalReply string
	oldSessionID := userState.SessionID

	newSessionID, modelName := callGemini(prompt, userState.SessionID, "", "", func(thought string, text string) {
		if text != "" {
			text = "🤖 *Reply:*\n" + text
		}
		finalReply = text
		editChan <- uiUpdate{thought: thought, text: text}
	})

	doneChan <- true
	cancelIndicator()

	if newSessionID != "" {
		userState.SessionID = newSessionID
	}

	modelSuffix := ""
	if modelName != "" {
		modelSuffix = fmt.Sprintf(" (%s)", modelName)
	}

	if oldSessionID == "" && newSessionID != "" {
		finalReply = fmt.Sprintf("🤖 gemini-cli-server v1.1.1\n🆔 Session: %s%s\n\n%s", newSessionID, modelSuffix, finalReply)
	}

	if finalReply == "" {
		finalReply = "Done."
	}

	// Final edit for the text message
	// If textMsgID was never created (no text arrived), edit the thought one or send new
	msgInfo := finalReply
	msg := tgbotapi.NewMessage(message.Chat.ID, msgInfo)
	msg.ParseMode = "Markdown"
	bot.Send(msg)
}

func handleSessionsCommand(message *tgbotapi.Message) {
	sessions, err := fetchSessions()
	if err != nil {
		msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("❌ Error fetching sessions: %v", err))
		bot.Send(msg)
		return
	}

	if len(sessions) == 0 {
		msg := tgbotapi.NewMessage(message.Chat.ID, "📭 No recent sessions found.")
		bot.Send(msg)
		return
	}

	startIdx := len(sessions) - 40
	if startIdx < 0 {
		startIdx = 0
	}

	activeSessions := sessions[startIdx:]
	
	chunkSize := 15
	for chunkStart := 0; chunkStart < len(activeSessions); chunkStart += chunkSize {
		chunkEnd := chunkStart + chunkSize
		if chunkEnd > len(activeSessions) {
			chunkEnd = len(activeSessions)
		}
		
		var sb strings.Builder
		if chunkStart == 0 {
			sb.WriteString(fmt.Sprintf("📋 *Recent Sessions (%d total):*\n\n", len(activeSessions)))
		} else {
			sb.WriteString("📋 *Recent Sessions (continued):*\n\n")
		}
		
		for i := chunkStart; i < chunkEnd; i++ {
			s := activeSessions[i]
			description := s.Description
			if len(description) > 50 {
				description = description[:47] + "..."
			}
			displayNum := i + 1 
			sb.WriteString(fmt.Sprintf("%d. %s\n   _Time: %s_\n   ID: `/attach %s`\n\n", displayNum, description, s.Time, s.ID))
		}
		
		msg := tgbotapi.NewMessage(message.Chat.ID, sb.String())
		msg.ParseMode = "Markdown"
		bot.Send(msg)
	}
}

func fetchSessions() ([]Session, error) {
	// Update URL for sessions endpoint
	sessionsURL := strings.Replace(geminiURL, "/event", "/sessions", 1)
	
	resp, err := http.Get(sessionsURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var sessResp SessionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&sessResp); err != nil {
		return nil, err
	}

	if !sessResp.Ok {
		return nil, fmt.Errorf(sessResp.Error)
	}

	return sessResp.Sessions, nil
}

func callGemini(prompt string, sessionId string, imageData string, mimeType string, onChunk func(string, string)) (string, string) {
	payload := GeminiPayload{
		Source:    "telegram",
		Message:   prompt,
		SessionID: sessionId,
		ImageData: imageData,
		MimeType:  mimeType,
		ApiKey:    geminiAPIKey,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "❌ Error", ""
	}

	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Post(geminiURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "❌ Request error", ""
	}
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)
	
	var fullText string
	var fullThought string
	var finalSession string
	var finalModel string
	
	for {
		var chunk StreamChunk
		if err := decoder.Decode(&chunk); err != nil {
			break
		}
		if chunk.Type == "init" {
			finalSession = chunk.SessionId
			finalModel = chunk.Model
		} else if chunk.Type == "error" {
			if chunk.Error != "" {
				fullText += "\n⚠️ " + chunk.Error
			}
			onChunk(fullThought, fullText)
		} else if chunk.Type == "thought" {
			if chunk.Subject != "" {
				fullThought += fmt.Sprintf("[%s] ", chunk.Subject)
			}
			fullThought += chunk.Content
			onChunk(fullThought, fullText)
		} else if chunk.Type == "tool_use" {
			fullThought += fmt.Sprintf("\n[🛠️ %s...]", chunk.ToolName)
			onChunk(fullThought, fullText)
		} else if chunk.Type == "message" && chunk.Role == "assistant" {
			fullText += chunk.Content
			onChunk(fullThought, fullText)
		}
	}
	
	if fullText == "" { fullText = "No reply." }

	// Post processing to remove thought on final return
	return finalSession, finalModel
}

func handleVoiceMessage(message *tgbotapi.Message) {
	// Check if we have an API key
	if geminiAPIKey == "" {
		msg := tgbotapi.NewMessage(message.Chat.ID,
			"🎤 Voice transcription requires a Gemini API key.\n\n"+
				"Please visit https://aistudio.google.com/apikey to generate a key, "+
				"then paste it here. Your key will be saved securely.\n\n"+
				"Or type 'cancel' to continue without voice transcription.")
		userState := getUserState(message.From.ID)
		userState.State = "waiting_api_key"
		bot.Send(msg)
		return
	}

	// Check voice message duration
	if message.Voice.Duration > maxVoiceDurationSeconds {
		minutes := maxVoiceDurationSeconds / 60
		msg := tgbotapi.NewMessage(message.Chat.ID,
			fmt.Sprintf("❌ Voice message too long. Maximum duration is %d minutes (%d seconds).\n"+
				"Your message is %d seconds long.", minutes, maxVoiceDurationSeconds, message.Voice.Duration))
		bot.Send(msg)
		return
	}

	// Check file size
	if message.Voice.FileSize > maxVoiceFileSizeBytes {
		sizeMB := maxVoiceFileSizeBytes / (1024 * 1024)
		msg := tgbotapi.NewMessage(message.Chat.ID,
			fmt.Sprintf("❌ Voice message file too large. Maximum size is %dMB.", sizeMB))
		bot.Send(msg)
		return
	}

	// Send typing indicator
	typingConfig := tgbotapi.NewChatAction(message.Chat.ID, tgbotapi.ChatTyping)
	bot.Send(typingConfig)

	text, err := transcribeVoice(message.Voice.FileID)
	if err != nil {
		msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("❌ Error transcribing voice: %v", err))
		bot.Send(msg)
		return
	}

	// If transcription is empty or very short, inform user
	if strings.TrimSpace(text) == "" {
		msg := tgbotapi.NewMessage(message.Chat.ID, "🎤 No speech detected in voice message.")
		bot.Send(msg)
		return
	}

	ctxText := ""
	if message.ReplyToMessage != nil {
		ctxText = fmt.Sprintf("Context: %s: %s\n\n",
			message.ReplyToMessage.From.FirstName,
			message.ReplyToMessage.Text)
	}
	prompt := fmt.Sprintf("%sYou are an assistant in a Telegram chat.\nAnswer this voice message (transcribed):\n\n%s: %s",
		ctxText, message.From.FirstName, text)

	userState := getUserState(message.From.ID)
	log.Printf("Processing voice message from %s (Current Session: %s): %s", message.From.UserName, userState.SessionID, text)

	// Send initial thinking message
	thinkingMsg := tgbotapi.NewMessage(message.Chat.ID, "Thinking...")
	if message.ReplyToMessage != nil {
		thinkingMsg.ReplyToMessageID = message.MessageID
	}
	sentMsg, err := bot.Send(thinkingMsg)
	if err != nil {
		log.Printf("Error sending thinking message: %v", err)
	}

	// Start typing indicator and UI updater
	indicatorCtx, cancelIndicator := context.WithCancel(context.Background())
	editChan, doneChan := startUIUpdater(indicatorCtx, message.Chat.ID, sentMsg.MessageID)

	var finalReply string
	oldSessionID := userState.SessionID
	newSessionID, modelName := callGemini(prompt, userState.SessionID, "", "", func(thought string, text string) {
		if text != "" {
			text = "🤖 *Reply:*\n" + text
		}
		finalReply = text
		editChan <- uiUpdate{thought: thought, text: text}
	})

	doneChan <- true
	
	cancelIndicator()

	if newSessionID != "" {
		userState.SessionID = newSessionID
	}

	modelSuffix := ""
	if modelName != "" {
		modelSuffix = fmt.Sprintf(" (%s)", modelName)
	}

	if oldSessionID == "" && newSessionID != "" {
		finalReply = fmt.Sprintf("🤖 gemini-cli-server v1.1.1\n🆔 Session: %s%s\n\n%s", newSessionID, modelSuffix, finalReply)
	}

	if finalReply == "" {
		finalReply = "Done."
	}

	msgInfo := finalReply
	msg := tgbotapi.NewMessage(message.Chat.ID, msgInfo)
	msg.ParseMode = "Markdown"
	bot.Send(msg)
}

func handlePhotoMessage(message *tgbotapi.Message) {
	// Get the largest photo
	photo := message.Photo[len(message.Photo)-1]
	
	imageData, err := downloadFileAsBase64(photo.FileID)
	if err != nil {
		msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("❌ Error downloading photo: %v", err))
		bot.Send(msg)
		return
	}

	prompt := message.Caption
	if prompt == "" {
		prompt = "What is in this image?"
	}

	userState := getUserState(message.From.ID)
	log.Printf("Processing photo from %s (Current Session: %s): %s", message.From.UserName, userState.SessionID, prompt)

	// Send initial thinking message
	thinkingMsg := tgbotapi.NewMessage(message.Chat.ID, "Thinking...")
	if message.ReplyToMessage != nil {
		thinkingMsg.ReplyToMessageID = message.MessageID
	}
	sentMsg, err := bot.Send(thinkingMsg)
	if err != nil {
		log.Printf("Error sending thinking message: %v", err)
	}

	// Start typing indicator and UI updater
	indicatorCtx, cancelIndicator := context.WithCancel(context.Background())
	editChan, doneChan := startUIUpdater(indicatorCtx, message.Chat.ID, sentMsg.MessageID)

	var finalReply string
	oldSessionID := userState.SessionID
	newSessionID, modelName := callGemini(prompt, userState.SessionID, imageData, "image/jpeg", func(thought string, text string) {
		if text != "" {
			text = "🤖 *Reply:*\n" + text
		}
		finalReply = text
		editChan <- uiUpdate{thought: thought, text: text}
	})

	doneChan <- true
	
	cancelIndicator()

	if newSessionID != "" {
		userState.SessionID = newSessionID
	}

	modelSuffix := ""
	if modelName != "" {
		modelSuffix = fmt.Sprintf(" (%s)", modelName)
	}

	if oldSessionID == "" && newSessionID != "" {
		finalReply = fmt.Sprintf("🤖 gemini-cli-server v1.1.1\n🆔 Session: %s%s\n\n%s", newSessionID, modelSuffix, finalReply)
	}

	if finalReply == "" {
		finalReply = "Done."
	}

	msgInfo := finalReply
	msg := tgbotapi.NewMessage(message.Chat.ID, msgInfo)
	msg.ParseMode = "Markdown"
	bot.Send(msg)
}

func downloadFileAsBase64(fileID string) (string, error) {
	file, err := bot.GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		return "", err
	}

	fileURL := file.Link(bot.Token)
	resp, err := http.Get(fileURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(data), nil
}

func handleAPIKeyInput(message *tgbotapi.Message) {
	text := strings.TrimSpace(message.Text)
	userState := getUserState(message.From.ID)

	if strings.ToLower(text) == "cancel" {
		userState.State = ""
		msg := tgbotapi.NewMessage(message.Chat.ID, "✅ Cancelled. You can continue chatting without voice transcription.")
		bot.Send(msg)
		return
	}

	// Validate API key format (basic validation)
	if !strings.HasPrefix(text, "AIza") || len(text) < 30 {
		msg := tgbotapi.NewMessage(message.Chat.ID,
			"❌ Invalid API key format. Gemini API keys typically start with 'AIza' and are longer.\n"+
				"Please check your key and try again, or type 'cancel' to skip.")
		bot.Send(msg)
		return
	}

	// Test the API key with a simple request
	if !testAPIKey(text) {
		msg := tgbotapi.NewMessage(message.Chat.ID,
			"❌ API key test failed. Please check that your key is valid .\n"+
				"Try again or type 'cancel' to skip.")
		bot.Send(msg)
		return
	}

	// Save API key to environment file
	if err := saveAPIKeyToEnv(text); err != nil {
		log.Printf("Error saving API key to .env: %v", err)
		msg := tgbotapi.NewMessage(message.Chat.ID,
			"⚠️ API key validated but failed to save to file. It will work for this session only.\n"+
				fmt.Sprintf("Error: %v", err))
		bot.Send(msg)
	} else {
		log.Printf("API key successfully saved to .env file")
	}

	// Update global API key and clear user state
	geminiAPIKey = text
	userState.State = ""

	msg := tgbotapi.NewMessage(message.Chat.ID, "✅ API key validated and saved! You can now send voice messages for transcription.")
	bot.Send(msg)

	// Delete the message containing the API key for security
	deleteMsg := tgbotapi.NewDeleteMessage(message.Chat.ID, message.MessageID)
	if _, err := bot.Request(deleteMsg); err != nil {
		log.Printf("Warning: Could not delete API key message: %v", err)
	}
}

func testAPIKey(apiKey string) bool {
	// Simple test with a minimal request
	payload := map[string]any{
		"contents": []map[string]any{
			{
				"parts": []map[string]any{
					{"text": "Hello"},
				},
			},
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return false
	}

	req, err := http.NewRequest("POST",
		"https://generativelanguage.googleapis.com/v1alpha/models/gemini-3.1-pro-preview:generateContent?key="+apiKey,
		bytes.NewBuffer(jsonData))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

func saveAPIKeyToEnv(apiKey string) error {
	// Read existing .env file content
	envContent := make(map[string]string)

	if data, err := os.ReadFile(envFilePath); err == nil {
		// Parse existing .env file
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				envContent[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}
	}

	// Update API key
	envContent["GEMINI_API_KEY"] = apiKey

	// Write back to file
	var envLines []string

	// Keep existing variables in order, update if exists
	if data, err := os.ReadFile(envFilePath); err == nil {
		lines := strings.Split(string(data), "\n")
		apiKeyWritten := false

		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				envLines = append(envLines, line)
				continue
			}

			parts := strings.SplitN(trimmed, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				if key == "GEMINI_API_KEY" {
					envLines = append(envLines, fmt.Sprintf("GEMINI_API_KEY=%s", apiKey))
					apiKeyWritten = true
				} else {
					envLines = append(envLines, line)
				}
			} else {
				envLines = append(envLines, line)
			}
		}

		// If API key wasn't in file, add it
		if !apiKeyWritten {
			envLines = append(envLines, fmt.Sprintf("GEMINI_API_KEY=%s", apiKey))
		}
	} else {
		// File doesn't exist, create new content
		envLines = append(envLines, fmt.Sprintf("GEMINI_API_KEY=%s", apiKey))
	}

	// Write to file
	return os.WriteFile(envFilePath, []byte(strings.Join(envLines, "\n")), 0o600)
}

func transcribeVoice(fileID string) (string, error) {
	file, err := bot.GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		return "", fmt.Errorf("failed to get file info: %w", err)
	}

	fileURL := file.Link(bot.Token)
	resp, err := http.Get(fileURL)
	if err != nil {
		return "", fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download file: status %d", resp.StatusCode)
	}

	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read audio data: %w", err)
	}

	// Determine MIME type based on file extension or content
	mimeType := "audio/ogg" // Default for Telegram voice messages
	if strings.Contains(file.FilePath, ".mp3") {
		mimeType = "audio/mpeg"
	} else if strings.Contains(file.FilePath, ".wav") {
		mimeType = "audio/wav"
	} else if strings.Contains(file.FilePath, ".m4a") {
		mimeType = "audio/mp4"
	}

	payload := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{
						"text": "Please transcribe this audio file accurately. Only return the transcribed text without any additional commentary.",
					},
					{
						"inline_data": map[string]interface{}{
							"mime_type": mimeType,
							"data":      base64.StdEncoding.EncodeToString(audioData),
						},
					},
				},
			},
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST",
		"https://generativelanguage.googleapis.com/v1alpha/models/gemini-3.1-pro-preview:generateContent?key="+geminiAPIKey,
		bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	apiResp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("API request failed: %w", err)
	}
	defer apiResp.Body.Close()

	if apiResp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(apiResp.Body)
		return "", fmt.Errorf("API returned status %d: %s", apiResp.StatusCode, string(bodyBytes))
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.NewDecoder(apiResp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if result.Error.Code != 0 {
		return "", fmt.Errorf("API error %d: %s", result.Error.Code, result.Error.Message)
	}

	if len(result.Candidates) > 0 && len(result.Candidates[0].Content.Parts) > 0 {
		return strings.TrimSpace(result.Candidates[0].Content.Parts[0].Text), nil
	}

	return "", fmt.Errorf("no transcription received from API")
}

func spawnRestart(scriptPath string, repoDir string) {
	cmd := exec.Command("/bin/bash", scriptPath, "restart")
	cmd.Dir = repoDir
	// Start the command but don't wait for it
	err := cmd.Start()
	if err != nil {
		log.Printf("Error starting restart script: %v", err)
	}
}

