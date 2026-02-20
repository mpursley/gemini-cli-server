package main

import (
	"bytes"
	"context"
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
}

type GeminiResponse struct {
	Reply     string `json:"reply"`
	SessionID string `json:"sessionId"`
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

func handler(rawEvt interface{}) {
	switch evt := rawEvt.(type) {
	case *events.Message:
		if evt.Info.IsFromMe {
			return
		}

		if targetJID != "" && !strings.Contains(evt.Info.Sender.String(), targetJID) {
			return
		}

		text := ""
		if msg := evt.Message.GetConversation(); msg != "" {
			text = msg
		} else if msg := evt.Message.GetExtendedTextMessage().GetText(); msg != "" {
			text = msg
		}

		if text == "" {
			return
		}

		jid := evt.Info.Sender.String()
		sessionID := userSessions[jid]
		
		log.Printf("Processing WhatsApp message from %s (Session: %s): %s", jid, sessionID, text)

		prompt := fmt.Sprintf("You are an assistant in a WhatsApp chat.\nAnswer this message:\n\n%s: %s", jid, text)
		
		reply, newSessionID := callGemini(prompt, sessionID)
		if newSessionID != "" {
			userSessions[jid] = newSessionID
		}

		if sessionID == "" && newSessionID != "" {
			reply = fmt.Sprintf("%s\n\n🆔 Session ID: %s", reply, newSessionID)
		}

		client.SendMessage(context.Background(), evt.Info.Chat, &waE2E.Message{
			Conversation: &reply,
		})
	}
}

func callGemini(prompt string, sessionId string) (string, string) {
	payload := GeminiPayload{
		Source:    "whatsapp",
		Message:   prompt,
		SessionID: sessionId,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshaling JSON: %v", err)
		return "❌ Error processing request", ""
	}

	httpClient := &http.Client{Timeout: 300 * time.Second}
	resp, err := httpClient.Post(geminiURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("Error calling Gemini: %v", err)
		return fmt.Sprintf("❌ Error from Gemini server: %v", err), ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Gemini returned status %d", resp.StatusCode)
		return fmt.Sprintf("❌ Gemini server error: %d", resp.StatusCode), ""
	}

	var geminiResp GeminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		log.Printf("Error decoding response: %v", err)
		return "❌ Error parsing response", ""
	}

	return geminiResp.Reply, geminiResp.SessionID
}
