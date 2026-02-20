#!/bin/bash

# Paths
REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$REPO_DIR/tmp"
WA_BOT_DIR="$REPO_DIR/whatsapp_bot"
WA_BOT_BIN="$WA_BOT_DIR/whatsapp_bot_bin"
WA_BOT_PID_FILE="$TMP_DIR/whatsapp-bot.pid"
WA_BOT_LOG="$TMP_DIR/whatsapp-bot.log"

mkdir -p "$TMP_DIR"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

stop_wa() {
    echo -e "${YELLOW}🛑 Stopping WhatsApp Bot...${NC}"
    
    if [ -f "$WA_BOT_PID_FILE" ]; then
        PID=$(cat "$WA_BOT_PID_FILE")
        if [ -n "$PID" ] && ps -p "$PID" > /dev/null; then
            kill "$PID"
            echo -e "  - Sent kill signal to PID $PID"
            sleep 1
        fi
        rm -f "$WA_BOT_PID_FILE"
    fi

    # Cleanup any orphan processes
    ORPHANS=$(pgrep -f "whatsapp_bot_bin")
    if [ -n "$ORPHANS" ]; then
        echo -e "  - Cleaning up orphan processes: $ORPHANS"
        pkill -f "whatsapp_bot_bin"
    fi
    
    echo -e "${GREEN}✅ WhatsApp Bot stopped.${NC}"
}

start_wa() {
    echo -e "${YELLOW}🚀 Starting WhatsApp Bot...${NC}"

    # 1. Compile
    echo -e "  - Compiling WhatsApp bot..."
    cd "$WA_BOT_DIR" || exit
    if ! go build -o whatsapp_bot_bin main.go >> "$WA_BOT_LOG" 2>&1; then
        echo -e "${RED}❌ Compilation failed! Check $WA_BOT_LOG${NC}"
        exit 1
    fi
    
    # 2. Check for QR code requirement (first run)
    if [ ! -f "whatsapp_bot.db" ]; then
        echo -e "${YELLOW}⚠️ First-time setup detected.${NC}"
        echo -e "   You need to scan a QR code. Please run the bot manually once:"
        echo -e "   ${GREEN}cd whatsapp_bot && ./whatsapp_bot_bin${NC}"
        echo -e "   After scanning, you can use this script to manage it in the background."
        exit 0
    fi

    # 3. Start in background
    echo -e "  - Starting in background..."
    nohup ./whatsapp_bot_bin >> "$WA_BOT_LOG" 2>&1 &
    echo $! > "$WA_BOT_PID_FILE"
    
    sleep 2
    if ps -p $(cat "$WA_BOT_PID_FILE") > /dev/null; then
        echo -e "${GREEN}✅ WhatsApp Bot started (PID $(cat "$WA_BOT_PID_FILE"))${NC}"
        echo -e "   Logs: $0 logs"
    else
        echo -e "${RED}❌ WhatsApp Bot failed to start!${NC}"
        echo -e "   Check logs: $0 logs"
    fi
}

status_wa() {
    if [ -f "$WA_BOT_PID_FILE" ]; then
        PID=$(cat "$WA_BOT_PID_FILE")
        if ps -p "$PID" > /dev/null; then
            echo -e "📊 WhatsApp Bot: ${GREEN}RUNNING${NC} (PID $PID)"
            return 0
        fi
    fi
    
    ORPHANS=$(pgrep -f "whatsapp_bot_bin")
    if [ -n "$ORPHANS" ]; then
        echo -e "📊 WhatsApp Bot: ${YELLOW}ORPHANED${NC} (PIDs: $ORPHANS)"
    else
        echo -e "📊 WhatsApp Bot: ${RED}STOPPED${NC}"
    fi
}

case "$1" in
    start)
        start_wa
        ;;
    stop)
        stop_wa
        ;;
    restart)
        stop_wa
        sleep 1
        start_wa
        ;;
    status)
        status_wa
        ;;
    logs)
        tail -f "$WA_BOT_LOG"
        ;;
    *)
        echo "Usage: $0 {start|stop|restart|status|logs}"
        exit 1
esac
