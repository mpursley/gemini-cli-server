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
        if [ -n "$PID" ]; then
            kill "$PID" >/dev/null 2>&1 || true
        fi
        rm -f "$BOT_PID_FILE"
    fi
    
    # Kill any other orphaned bot processes
    pkill -f "telegram_bot_bin" > /dev/null 2>&1
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
    mkdir -p /tmp
    nohup node "$LISTEN_SCRIPT" 8765 >> /tmp/gemini-listen.log 2>&1 &
    echo $! > "$LISTEN_PID_FILE"
    sleep 2 # Give it a moment to bind to port
    
    if lsof -i :8765 > /dev/null; then
        echo "  - Listener started (PID $(cat "$LISTEN_PID_FILE"))"
    else
        echo "  - ❌ Listener failed to start! Check /tmp/gemini-listen.log"
    fi

    # 2. Start bot
    if [ ! -f "$BOT_BIN" ]; then
        echo "  - Compiling bot..."
        cd "$BOT_DIR" && go build -o telegram_bot_bin main.go
        cd "$REPO_DIR"
    fi
    
    echo "  - Starting Telegram bot..."
    cd "$BOT_DIR"
    nohup ./telegram_bot_bin >> /tmp/telegram-bot.log 2>&1 &
    echo $! > "$BOT_PID_FILE"
    cd "$REPO_DIR"
    echo "  - Telegram Bot started (PID $(cat "$BOT_PID_FILE"))"

    echo "✅ Services are up."
}

status_all() {
    echo "📊 Status:"
    if lsof -i :8765 > /dev/null; then 
        echo "  - Listener: RUNNING (on port 8765)"
    else 
        echo "  - Listener: STOPPED"
    fi
    
    if pgrep -f "telegram_bot_bin" > /dev/null; then 
        echo "  - Bot:      RUNNING"
    else 
        echo "  - Bot:      STOPPED"
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
    *)
        echo "Usage: $0 {start|stop|restart|status}"
        exit 1
esac
