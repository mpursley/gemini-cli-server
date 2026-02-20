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
	userSessions = make(map[string]string) // Map JID to SessionID
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found")
	}

	geminiURL = os.Getenv("GEMINI_ENDPOINT")
	if geminiURL == "" {
		geminiURL = "http://127.0.0.1:8765/event"
	}
	targetJID = os.Getenv("TARGET_JID")

	dbLog := waLog.Stdout("Database", "DEBUG", true)
	container, err := sqlstore.New(context.Background(), "sqlite3", "file:whatsapp_bot.db?_foreign_keys=on", dbLog)
	if err != nil {
		log.Fatal(err)
	}

	deviceStore, err := container.GetFirstDevice(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	clientLog := waLog.Stdout("Client", "DEBUG", true)
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
		err = client.Connect()
		if err != nil {
			log.Fatal(err)
		}
	}

	// Listen to system signals to safely shut down
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	client.Disconnect()
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
	case *events.Message:
		if evt.Info.IsFromMe {
			return
		}

		if targetJID != "" && !strings.Contains(evt.Info.Sender.String(), targetJID) {
			return
		}

		jid := evt.Info.Sender.String()
		sessionID := userSessions[jid]

		// 1. Handle Images
		var imageData, mimeType string
		if img := evt.Message.GetImageMessage(); img != nil {
			data, err := client.Download(img)
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
				client.SendMessage(context.Background(), evt.Info.Chat, &waE2E.Message{Conversation: strings.Ptr("🆕 Started a new session.")})
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

		reply, newSessionID, modelName := callGemini(prompt, sessionID, imageData, mimeType)
		if newSessionID != "" {
			userSessions[jid] = newSessionID
		}

		if sessionID == "" && newSessionID != "" {
			modelSuffix := ""
			if modelName != "" {
				modelSuffix = fmt.Sprintf(" (%s)", modelName)
			}
			reply = fmt.Sprintf("%s\n\n🆔 Session: %s%s", reply, newSessionID, modelSuffix)
		}

		client.SendMessage(context.Background(), evt.Info.Chat, &waE2E.Message{
			Conversation: &reply,
		})
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
		if i >= 10 {
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

func callGemini(prompt string, sessionId string, imageData string, mimeType string) (string, string, string) {
	payload := GeminiPayload{
		Source:    "whatsapp",
		Message:   prompt,
		SessionID: sessionId,
		ImageData: imageData,
		MimeType:  mimeType,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshaling JSON: %v", err)
		return "❌ Error processing request", "", ""
	}

	httpClient := &http.Client{Timeout: 300 * time.Second}
	resp, err := httpClient.Post(geminiURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("Error calling Gemini: %v", err)
		return fmt.Sprintf("❌ Error from Gemini server: %v", err), "", ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Gemini returned status %d", resp.StatusCode)
		return fmt.Sprintf("❌ Gemini server error: %d", resp.StatusCode), "", ""
	}

	var geminiResp GeminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		log.Printf("Error decoding response: %v", err)
		return "❌ Error parsing response", "", ""
	}

	return geminiResp.Reply, geminiResp.SessionID, geminiResp.Model
}
