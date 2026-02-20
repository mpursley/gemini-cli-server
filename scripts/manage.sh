#!/bin/bash

# Paths
REPO_DIR="/Users/mpursley/dev/gemini_cli_server"
BOT_DIR="$REPO_DIR/telegram_bot"
BOT_BIN="$BOT_DIR/telegram_bot_bin"
BOT_PID_FILE="/tmp/telegram-bot.pid"
LISTEN_PID_FILE="/tmp/gemini-listen.pid"
LISTEN_SCRIPT="$HOME/.gemini/scripts/listen.js"

stop_all() {
    echo "🛑 Stopping services..."
    
    # Stop Telegram Bot
    if [ -f "$BOT_PID_FILE" ]; then
        PID=$(cat "$BOT_PID_FILE")
        if ps -p "$PID" > /dev/null 2>&1; then
            kill "$PID"
            echo "  - Stopped bot (PID $PID)"
        fi
        rm -f "$BOT_PID_FILE"
    fi
    
    # Kill any other orphaned bot processes
    pkill -f "telegram_bot_bin" > /dev/null 2>&1
    pkill -f "go run main.go" > /dev/null 2>&1
    
    # Stop Listener
    if [ -f "$LISTEN_PID_FILE" ]; then
        PID=$(cat "$LISTEN_PID_FILE")
        if ps -p "$PID" > /dev/null 2>&1; then
            kill "$PID"
            echo "  - Stopped listener (PID $PID)"
        fi
        rm -f "$LISTEN_PID_FILE"
    fi
    
    # Kill any other orphaned listener processes
    pkill -f "node.*listen.js" > /dev/null 2>&1
    
    echo "✅ All services stopped."
}

start_all() {
    echo "🚀 Starting services..."

    # 1. Check if listener is running
    if pgrep -f "node.*listen.js" > /dev/null; then
        echo "  - ⚠️ Listener is already running. Skipping start."
    else
        echo "  - Starting listener..."
        mkdir -p /tmp
        nohup node "$LISTEN_SCRIPT" 8765 >> /tmp/gemini-listen.log 2>&1 &
        echo $! > "$LISTEN_PID_FILE"
        echo "  - Listener started (PID $!)"
    fi

    # 2. Check if bot is running
    if pgrep -f "telegram_bot_bin" > /dev/null || pgrep -f "go run main.go" > /dev/null; then
        echo "  - ⚠️ Telegram Bot is already running. Skipping start."
    else
        # Ensure binary exists
        if [ ! -f "$BOT_BIN" ]; then
            echo "  - Compiling bot..."
            cd "$BOT_DIR" && go build -o telegram_bot_bin main.go
        fi
        
        echo "  - Starting Telegram bot..."
        cd "$BOT_DIR"
        nohup ./telegram_bot_bin >> /tmp/telegram-bot.log 2>&1 &
        echo $! > "$BOT_PID_FILE"
        echo "  - Telegram Bot started (PID $!)"
    fi

    echo "✅ Services are up."
    echo "   Logs: tail -f /tmp/gemini-listen.log /tmp/telegram-bot.log"
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
        echo "📊 Status:"
        if pgrep -f "node.*listen.js" > /dev/null; then echo "  - Listener: RUNNING"; else echo "  - Listener: STOPPED"; fi
        if pgrep -f "telegram_bot_bin" > /dev/null || pgrep -f "go run main.go" > /dev/null; then echo "  - Bot:      RUNNING"; else echo "  - Bot:      STOPPED"; fi
        ;;
    *)
        echo "Usage: $0 {start|stop|restart|status}"
        exit 1
esac
