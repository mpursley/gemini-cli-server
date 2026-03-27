package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
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

var (
	client       *whatsmeow.Client
	geminiURL    string
	targetJID    string
	geminiAPIKey string
	userSessions = make(map[string]string) // Map JID to SessionID
)

func strPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}

func initVars() {
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found or error loading it. Relying on environment variables.")
	}

	geminiURL = os.Getenv("GEMINI_ENDPOINT")
	if geminiURL == "" {
		geminiURL = "http://127.0.0.1:8765/event"
	}
	
	geminiAPIKey = os.Getenv("GEMINI_API_KEY")

	targetJID = os.Getenv("TARGET_JID")
	if targetJID == "" {
		log.Fatal("TARGET_JID is missing. Please set it in your .env or as an environment variable.")
	}
}

func main() {
	initVars()

	dbLog := waLog.Stdout("Database", "INFO", true)
	container, err := sqlstore.New(context.Background(), "sqlite3", "file:whatsapp_bot.db?_foreign_keys=on", dbLog)
	if err != nil {
		log.Fatal(err)
	}

	deviceStore, err := container.GetFirstDevice(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	clientLog := waLog.Stdout("Client", "INFO", true)
	client = whatsmeow.NewClient(deviceStore, clientLog)
	client.AddEventHandler(handler)

	if client.Store.ID == nil {
		qrChan, _ := client.GetQRChannel(context.Background())
		err = client.Connect()
		if err != nil {
			log.Fatal(err)
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
			} else {
				fmt.Println("QR channel result:", evt.Event)
			}
		}
	} else {
		// Already logged in, just connect
		err = client.Connect()
		if err != nil {
			log.Fatal(err)
		}
	}

	// Keep-alive/reconnect loop
	go func() {
		for {
			time.Sleep(10 * time.Second)
			if !client.IsConnected() {
				log.Printf("WhatsApp client disconnected, attempting to reconnect...")
				err := client.Connect()
				if err != nil {
					log.Printf("Failed to reconnect: %v", err)
				} else {
					log.Printf("Successfully reconnected to WhatsApp")
				}
			}
		}
	}()

	// Listen to system signals to safely shut down
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	client.Disconnect()
}

func typingIndicator(ctx context.Context, chat types.JID) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Set back to paused/not composing when finished
			client.SendChatPresence(context.Background(), chat, types.ChatPresencePaused, types.ChatPresenceMediaText)
			return
		case <-ticker.C:
			// Refresh the "composing" status
			client.SendChatPresence(context.Background(), chat, types.ChatPresenceComposing, types.ChatPresenceMediaText)
		}
	}
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

func handler(rawEvt interface{}) {
	switch evt := rawEvt.(type) {
	case *events.Connected:
		log.Printf("WhatsApp bot connected and ready. My JID: %s", client.Store.ID.String())
	case *events.Message:
		// Process message in a goroutine so we don't block the main event loop
		// and avoid the "Node handling is taking long" warning/timeout.
		go func(evt *events.Message) {
			// Debug logging to understand why messages might be ignored
			sender := evt.Info.Sender.String()

			// If it's from us, usually we ignore. 
			// BUT if it's from the user's LID account (even if marked FromMe), we want to process it.
			isUserLID := strings.Contains(sender, "229712718741576")
			
			if evt.Info.IsFromMe && !isUserLID {
				return
			}

			// Also ignore edits to avoid responding to our own "Thinking..." updates
			// (Unless it's from the user's LID, but usually users don't "edit" into a loop)
			if evt.Message.GetProtocolMessage().GetType() == waE2E.ProtocolMessage_MESSAGE_EDIT && !isUserLID {
				return
			}

			if targetJID != "" && !strings.Contains(sender, targetJID) {
				log.Printf("Ignoring message from %s (targetJID filter)", sender)
				return
			}

			jid := sender
			sessionID := userSessions[jid]

			// 1. Handle Images
			var imageData, mimeType string
			if img := evt.Message.GetImageMessage(); img != nil {
				data, err := client.Download(context.Background(), img)
				if err != nil {
					log.Printf("Failed to download image: %v", err)
				} else {
					imageData = base64.StdEncoding.EncodeToString(data)
					mimeType = img.GetMimetype()
				}
			}

			// 2. Handle Text
			text := ""
			if msg := evt.Message.GetConversation(); msg != "" {
				text = msg
			} else if msg := evt.Message.GetExtendedTextMessage().GetText(); msg != "" {
				text = msg
			} else if msg := evt.Message.GetImageMessage().GetCaption(); msg != "" {
				text = msg
			}

			if text == "" && imageData == "" {
				return
			}

			// 3. Handle Commands
			if strings.HasPrefix(text, "/") {
				parts := strings.Fields(text)
				cmd := parts[0]
				switch cmd {
				case "/new":
					userSessions[jid] = ""
					client.SendMessage(context.Background(), evt.Info.Chat, &waE2E.Message{Conversation: strPtr("🆕 Started a new session.")})
					return
				case "/status":
					sID := "None"
					if sessionID != "" {
						sID = sessionID
					}
					statusMsg := fmt.Sprintf("📊 *WhatsApp Bot Status*\n\n🔗 Session: %s", sID)
					client.SendMessage(context.Background(), evt.Info.Chat, &waE2E.Message{Conversation: &statusMsg})
					return
				case "/sessions":
					handleSessionsCommand(evt)
					return
				case "/attach":
					if len(parts) < 2 {
						msg := "❌ Please provide a session ID. Example: /attach 8a3d..."
						client.SendMessage(context.Background(), evt.Info.Chat, &waE2E.Message{Conversation: &msg})
						return
					}
					userSessions[jid] = parts[1]
					msg := fmt.Sprintf("🔗 Attached to session: %s", parts[1])
					client.SendMessage(context.Background(), evt.Info.Chat, &waE2E.Message{Conversation: &msg})
					return
				}
			}

			log.Printf("Processing WhatsApp message from %s (Session: %s): %s", jid, sessionID, text)

			prompt := fmt.Sprintf("You are an assistant in a WhatsApp chat.\nAnswer this message:\n\n%s: %s", jid, text)
			if text == "" && imageData != "" {
				prompt = "What is in this image?"
			}

			// Start persistent typing indicator
			indicatorCtx, cancelIndicator := context.WithCancel(context.Background())
			go typingIndicator(indicatorCtx, evt.Info.Chat)

			// Debounced UI updater
			type uiUpdate struct {
				thought string
				text    string
			}
			editChan := make(chan uiUpdate, 100)
			doneChan := make(chan bool)
			go func() {
				var lastEdit time.Time
				var lastThought string
				var lastText string
				var thoughtMsgID string
				var textMsgID string

				timer := time.NewTimer(time.Hour)
				timer.Stop()

				for {
					select {
					case update := <-editChan:
						if time.Since(lastEdit) > 2*time.Second {
							if update.thought != lastThought && update.thought != "" {
								msgInfo := "💭 *Thinking:*\n" + update.thought
								if thoughtMsgID == "" {
									sentMsg, _ := client.SendMessage(context.Background(), evt.Info.Chat, &waE2E.Message{
										Conversation: &msgInfo,
									})
									if sentMsg.ID != "" {
										thoughtMsgID = sentMsg.ID
									}
								} else {
									client.SendMessage(context.Background(), evt.Info.Chat, client.BuildEdit(evt.Info.Chat, thoughtMsgID, &waE2E.Message{
										Conversation: &msgInfo,
									}))
								}
								lastThought = update.thought
							}
							if update.text != lastText && update.text != "" {
								if textMsgID == "" {
									sentMsg, _ := client.SendMessage(context.Background(), evt.Info.Chat, &waE2E.Message{
										Conversation: &update.text,
									})
									if sentMsg.ID != "" {
										textMsgID = sentMsg.ID
									}
								} else {
									client.SendMessage(context.Background(), evt.Info.Chat, client.BuildEdit(evt.Info.Chat, textMsgID, &waE2E.Message{
										Conversation: &update.text,
									}))
								}
								lastText = update.text
							}
							lastEdit = time.Now()
							timer.Stop()
						} else {
							timer.Reset(2 * time.Second)
						}
					case <-timer.C:
						if len(editChan) > 0 {
							// For simplistic flushing
						}
					case <-doneChan:
						timer.Stop()
						return
					}
				}
			}()

			var finalReply string
			newSessionID, modelName := callGemini(prompt, sessionID, imageData, mimeType, func(thought string, text string) {
				finalReply = text
				editChan <- uiUpdate{thought: thought, text: text}
			})
			
			doneChan <- true
			
			// Stop the indicator before sending the reply
			cancelIndicator()
			
			if newSessionID != "" {
				userSessions[jid] = newSessionID
			}

			if sessionID == "" && newSessionID != "" {
				modelSuffix := ""
				if modelName != "" {
					modelSuffix = fmt.Sprintf(" (%s)", modelName)
				}
				finalReply = fmt.Sprintf("🤖 gemini-cli-server v1.1.1\n🆔 Session: %s%s\n\n%s", newSessionID, modelSuffix, finalReply)
			}

			if finalReply == "" {
				finalReply = "Done."
			}

			client.SendMessage(context.Background(), evt.Info.Chat, &waE2E.Message{
				Conversation: &finalReply,
			})
		}(evt)
	}
}

func handleSessionsCommand(evt *events.Message) {
	sessions, err := fetchSessions()
	if err != nil {
		msg := fmt.Sprintf("❌ Error fetching sessions: %v", err)
		client.SendMessage(context.Background(), evt.Info.Chat, &waE2E.Message{Conversation: &msg})
		return
	}

	if len(sessions) == 0 {
		msg := "📭 No recent sessions found."
		client.SendMessage(context.Background(), evt.Info.Chat, &waE2E.Message{Conversation: &msg})
		return
	}

	var sb strings.Builder
	sb.WriteString("📋 *Recent Sessions:*\n\n")
	for i, s := range sessions {
		if i >= 15 {
			break
		}
		desc := s.Description
		if len(desc) > 50 {
			desc = desc[:47] + "..."
		}
		sb.WriteString(fmt.Sprintf("%d. %s\n   Time: %s\n   ID: /attach %s\n\n", i+1, desc, s.Time, s.ID))
	}

	reply := sb.String()
	client.SendMessage(context.Background(), evt.Info.Chat, &waE2E.Message{Conversation: &reply})
}

func fetchSessions() ([]Session, error) {
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
		Source:    "whatsapp",
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

	httpClient := &http.Client{Timeout: 30 * time.Minute}
	resp, err := httpClient.Post(geminiURL, "application/json", bytes.NewBuffer(jsonData))
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

	return finalSession, finalModel
}
