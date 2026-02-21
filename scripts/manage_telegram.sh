#!/bin/bash

# Paths
REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$REPO_DIR/tmp"
BOT_DIR="$REPO_DIR/telegram_bot"
BOT_BIN="$BOT_DIR/telegram_bot_bin"
BOT_PID_FILE="$TMP_DIR/telegram-bot.pid"
BOT_LOG="$TMP_DIR/telegram-bot.log"

WA_BOT_DIR="$REPO_DIR/whatsapp_bot"
WA_BOT_BIN="$WA_BOT_DIR/whatsapp_bot_bin"
WA_BOT_PID_FILE="$TMP_DIR/whatsapp-bot.pid"
WA_BOT_LOG="$TMP_DIR/whatsapp-bot.log"

LISTEN_PID_FILE="$TMP_DIR/gemini-listen.pid"
LISTEN_LOG="$TMP_DIR/gemini-listen.log"
LISTEN_SCRIPT="$REPO_DIR/scripts/listen.js"

mkdir -p "$TMP_DIR"

stop_all() {
    echo "🛑 Stopping services..."
    
    # Stop Telegram Bot
    if [ -f "$BOT_PID_FILE" ]; then
        PID=$(cat "$BOT_PID_FILE")
        if [ -n "$PID" ]; then
            kill "$PID" >/dev/null 2>&1 || true
        fi
        rm -f "$BOT_PID_FILE"
    fi

    # Stop WhatsApp Bot
    if [ -f "$WA_BOT_PID_FILE" ]; then
        PID=$(cat "$WA_BOT_PID_FILE")
        if [ -n "$PID" ]; then
            kill "$PID" >/dev/null 2>&1 || true
        fi
        rm -f "$WA_BOT_PID_FILE"
    fi
    
    # Kill any other orphaned bot processes
    pkill -f "telegram_bot_bin" > /dev/null 2>&1
    pkill -f "whatsapp_bot_bin" > /dev/null 2>&1
    pkill -f "go run main.go" > /dev/null 2>&1
    
    # Stop Listener
    if [ -f "$LISTEN_PID_FILE" ]; then
        PID=$(cat "$LISTEN_PID_FILE")
        if [ -n "$PID" ]; then
            kill "$PID" >/dev/null 2>&1 || true
        fi
        rm -f "$LISTEN_PID_FILE"
    fi
    
    # Kill any other orphaned listener processes
    pkill -f "listen.js" > /dev/null 2>&1
    
    # Force kill anything on port 8765
    LSOF_PID=$(lsof -t -i :8765)
    if [ -n "$LSOF_PID" ]; then
        echo "  - Clearing port 8765 (PID $LSOF_PID)"
        kill -9 $LSOF_PID >/dev/null 2>&1 || true
    fi
    
    echo "✅ All services stopped."
}

start_all() {
    echo "🚀 Starting services..."

    # 1. Start listener
    echo "  - Starting listener..."
    nohup node "$LISTEN_SCRIPT" 8765 >> "$LISTEN_LOG" 2>&1 &
    echo $! > "$LISTEN_PID_FILE"
    sleep 2 # Give it a moment to bind to port
    
    if lsof -i :8765 > /dev/null; then
        echo "  - Listener started (PID $(cat "$LISTEN_PID_FILE"))"
    else
        echo "  - ❌ Listener failed to start! Check $LISTEN_LOG"
    fi

    # 2. Start Telegram bot
    echo "  - Compiling Telegram bot..."
    cd "$BOT_DIR"
    if go build -o telegram_bot_bin main.go >> "$BOT_LOG" 2>&1; then
        echo "  - Telegram Bot compiled successfully."
    else
        echo "  - ❌ Telegram Bot compilation failed! Check $BOT_LOG"
        cd "$REPO_DIR"
        return 1
    fi
    cd "$REPO_DIR"
    
    echo "  - Starting Telegram bot..."
    cd "$BOT_DIR"
    nohup ./telegram_bot_bin >> "$BOT_LOG" 2>&1 &
    echo $! > "$BOT_PID_FILE"
    cd "$REPO_DIR"
    echo "  - Telegram Bot started (PID $(cat "$BOT_PID_FILE"))"

    # 3. Start WhatsApp bot (Optional/Manual QR)
    if [ "$START_WA" = "true" ]; then
        echo "  - Compiling WhatsApp bot..."
        cd "$WA_BOT_DIR"
        if go build -o whatsapp_bot_bin main.go >> "$WA_BOT_LOG" 2>&1; then
            echo "  - WhatsApp Bot compiled successfully."
        else
            echo "  - ❌ WhatsApp Bot compilation failed! Check $WA_BOT_LOG"
            cd "$REPO_DIR"
            return 1
        fi
        cd "$REPO_DIR"
        
        echo "  - Starting WhatsApp bot..."
        echo "    NOTE: If this is the first time, you may need to run it manually to scan the QR code:"
        echo "    cd whatsapp_bot && ./whatsapp_bot_bin"
        cd "$WA_BOT_DIR"
        nohup ./whatsapp_bot_bin >> "$WA_BOT_LOG" 2>&1 &
        echo $! > "$WA_BOT_PID_FILE"
        cd "$REPO_DIR"
        echo "  - WhatsApp Bot started (PID $(cat "$WA_BOT_PID_FILE"))"
    fi

    echo "✅ Services are up."
    echo "   To view logs, run: $0 logs"
}

status_all() {
    echo "📊 Status:"
    if lsof -i :8765 > /dev/null; then 
        echo "  - Listener: RUNNING (on port 8765)"
    else 
        echo "  - Listener: STOPPED"
    fi
    
    if pgrep -f "telegram_bot_bin" > /dev/null; then 
        echo "  - Telegram Bot: RUNNING"
    else 
        echo "  - Telegram Bot: STOPPED"
    fi

    if pgrep -f "whatsapp_bot_bin" > /dev/null; then 
        echo "  - WhatsApp Bot: RUNNING"
    else 
        echo "  - WhatsApp Bot: STOPPED"
    fi
}

case "$1" in
    start)
        start_all
        ;;
    stop)
        stop_all
        ;;
    restart)
        stop_all
        sleep 1
        start_all
        ;;
    status)
        status_all
        ;;
    logs)
        tail -f "$LISTEN_LOG" "$BOT_LOG" 2>/dev/null
        ;;
    *)
        echo "Usage: $0 {start|stop|restart|status|logs}"
        exit 1
esac
