#!/usr/bin/env node
const http = require("http");
const fs = require("fs");
const path = require("path");
const { spawn, spawnSync } = require("child_process");

const port = Number(process.argv[2] || process.env.PORT || 8765);



function runGemini(prompt, sessionId = null, imageData = null, mimeType = null, apiKey = null, res) {
  let tempFile = null;
  let args = ["--yolo", "-m", "gemini-3.1-pro-preview", "--output-format", "stream-json"];
  
  // Special handling for /resume save <name>
  const resumeSaveMatch = prompt.match(/\/resume save (.+)/);
  if (resumeSaveMatch) {
    const sessionName = resumeSaveMatch[1].trim();
    if (sessionName && sessionId) {
      args = ["--resume", sessionId, "--save-session", sessionName];
      console.log(`[${new Date().toISOString()}] Saving session ${sessionId} as "${sessionName}"`);
      
      const envVars = Object.assign({}, process.env);
      if (apiKey) envVars.GEMINI_API_KEY = apiKey;
      
      let cliCommand = "gemini";
      let cliArgs = args;
      const localDevBundle = process.env.HOME + "/dev/gemini-cli/bundle/gemini.js";
      if (fs.existsSync(localDevBundle)) {
        cliCommand = "node";
        cliArgs = [localDevBundle, ...args];
      }
      
      const p = spawnSync(cliCommand, cliArgs, { env: envVars, encoding: "utf8" });
      res.writeHead(200, { "Content-Type": "application/x-ndjson" });
      if (p.status === 0) {
        res.write(JSON.stringify({ type: "message", role: "assistant", content: `✅ Session saved as: ${sessionName}` }) + "\n");
      } else {
        res.write(JSON.stringify({ type: "error", error: p.stderr || "Failed to save session" }) + "\n");
      }
      res.end();
      return;
    }
  }

  if (sessionId) {
    args.push("--resume", sessionId);
  }
  
  let finalPrompt = prompt;
  if (imageData) {
    const ext = mimeType === "image/png" ? "png" : "jpg";
    const fileName = `upload-${Date.now()}.${ext}`;
    tempFile = path.join(__dirname, "..", "uploads", fileName);
    fs.writeFileSync(tempFile, Buffer.from(imageData, "base64"));
    finalPrompt = `${prompt} ${tempFile}`;
  }
  
  args.push("--prompt", finalPrompt);
  console.log(`[${new Date().toISOString()}] Executing Gemini Streaming (Session: ${sessionId || "new"})${tempFile ? ` with image` : ""}`);
  
  const envVars = Object.assign({}, process.env);
  if (apiKey) {
    envVars.GEMINI_API_KEY = apiKey;
  }

  let cliCommand = "gemini";
  let cliArgs = args;
  
  // Prefer the explicitly patched local repository if it exists
  const localDevBundle = process.env.HOME + "/dev/gemini-cli/bundle/gemini.js";
  if (fs.existsSync(localDevBundle)) {
    cliCommand = "node";
    cliArgs = [localDevBundle, ...args];
  }

  const p = spawn(cliCommand, cliArgs, { stdio: ["ignore", "pipe", "pipe"], env: envVars });
  const readline = require("readline");
  const rl = readline.createInterface({ input: p.stdout });
  
  res.writeHead(200, {
    "Content-Type": "application/x-ndjson",
    "Cache-Control": "no-cache",
    "Connection": "keep-alive"
  });

  let currentSessionId = sessionId;
  let userPromptLogged = false;

  const getLogPath = (sid) => {
    const logsDir = path.join(__dirname, "..", "logs", "sessions");
    if (!fs.existsSync(logsDir)) {
      fs.mkdirSync(logsDir, { recursive: true });
    }
    return path.join(logsDir, `${sid}.txt`);
  };

  const getLogPrefix = () => {
    const dateStr = new Date().toISOString().split('T')[0];
    return `${dateStr} :`;
  };

  const logToFile = (sid, typeStr, textStr) => {
    if (!textStr) return;
    const line = `${getLogPrefix()} ${typeStr} : ${textStr.replace(/\n/g, ' ')}\n`;
    fs.appendFileSync(getLogPath(sid), line);
  };

  rl.on("line", (line) => {
    if (!line.trim()) return;
    res.write(line + "\n");
    
    try {
      const parsed = JSON.parse(line);
      
      // Update session ID if it was newly created
      if (parsed.type === "init" && parsed.sessionId) {
        currentSessionId = parsed.sessionId;
      }
      
      if (currentSessionId) {
        // Log the user's prompt once we have a session ID
        if (!userPromptLogged) {
          logToFile(currentSessionId, "User", prompt);
          userPromptLogged = true;
        }
        
        let typeStr = "";
        let textStr = "";
        
        if (parsed.type === "message" && parsed.role === "assistant") {
           typeStr = "Assistant";
           textStr = parsed.content;
        } else if (parsed.type === "thought") {
           typeStr = "Thought";
           textStr = (parsed.subject ? `[${parsed.subject}] ` : "") + parsed.content;
        } else if (parsed.type === "tool_use") {
           typeStr = "Tool Use";
           textStr = `Executing tool ${parsed.toolName}`;
        } else if (parsed.type === "error") {
           typeStr = "Error";
           textStr = parsed.error || "Unknown error";
        }
        
        if (typeStr) {
           logToFile(currentSessionId, typeStr, textStr);
        }
      }
    } catch (e) {
      // Ignore JSON parse errors for incomplete or non-JSON lines
    }
  });

  let errText = "";
  p.stderr.on("data", d => (errText += d.toString()));
  p.on("close", code => {
    if (code !== 0 && code !== null && errText) {
      res.write(JSON.stringify({ type: "error", error: errText, status: "error" }) + "\n");
    }
    res.end();
  });
}

function listSessions() {
  return new Promise((resolve, reject) => {
    console.log(`[${new Date().toISOString()}] Listing sessions...`);
    let cliCommand = "gemini";
    let cliArgs = ["--list-sessions"];
    const localDevBundle = process.env.HOME + "/dev/gemini-cli/bundle/gemini.js";
    if (fs.existsSync(localDevBundle)) {
      cliCommand = "node";
      cliArgs = [localDevBundle, "--list-sessions"];
    }

    const p = spawn(cliCommand, cliArgs, { stdio: ["ignore", "pipe", "pipe"] });
    let out = "", err = "";
    p.stdout.on("data", d => (out += d.toString()));
    p.stderr.on("data", d => (err += d.toString()));
    p.on("close", code => {
      if (code === 0 || code === null) {
        const lines = out.split("\n");
        const sessions = [];
        
        for (const line of lines) {
          const trimmed = line.trim();
          if (!trimmed) continue;
          // Match digits, then capture description, time ago, and uuid.
          const match = trimmed.match(/(\d+)\.\s+(.*?)\s+\((.*?)\)\s+\[(.*?)\]$/);
          if (match) {
            sessions.push({
              description: match[2].trim(),
              time: match[3].trim(),
              id: match[4].trim()
            });
          }
        }
        console.log(`[${new Date().toISOString()}] Found ${sessions.length} sessions.`);
        resolve(sessions);
      } else {
        console.error(`[${new Date().toISOString()}] Error listing sessions: ${err}`);
        reject(new Error(err || `exit ${code}`));
      }
    });
  });
}

const server = http.createServer((req, res) => {
  if (req.method === "GET" && req.url === "/health") {
    res.writeHead(200, { "Content-Type": "text/plain" });
    return res.end("ok");
  }

  if (req.method === "GET" && req.url === "/sessions") {
    listSessions()
      .then(sessions => {
        res.writeHead(200, { "Content-Type": "application/json" });
        res.end(JSON.stringify({ ok: true, sessions }));
      })
      .catch(e => {
        res.writeHead(500, { "Content-Type": "application/json" });
        res.end(JSON.stringify({ ok: false, error: String(e.message || e) }));
      });
    return;
  }

  if (req.method === "POST" && req.url.startsWith("/event")) {
    let raw = "";
    req.on("data", c => (raw += c));
    req.on("end", async () => {
      let parsed;
      try {
        parsed = JSON.parse(raw || "{}");
      } catch {
        parsed = { raw };
      }

      const source = parsed.source || "unknown";
      const message = parsed.message || "";
      const sessionId = parsed.sessionId || null;
      const imageData = parsed.imageData || null;
      const mimeType = parsed.mimeType || null;
      const apiKey = parsed.apiKey || null;

      if (!message && !imageData) {
        res.writeHead(400, { "Content-Type": "application/json" });
        return res.end(JSON.stringify({ ok: false, error: "No message or image provided" }));
      }

      runGemini(message, sessionId, imageData, mimeType, apiKey, res);
    });
    return;
  }

  res.writeHead(404, { "Content-Type": "text/plain" });
  res.end("Not found");
});

// Increase server timeouts for long-running Gemini tasks
server.timeout = 1800000; // 30 minutes
server.headersTimeout = 1800000; // 30 minutes
server.requestTimeout = 1800000; // 30 minutes

function cleanup() {
  server.close(() => process.exit(0));
  setTimeout(() => process.exit(0), 1500).unref();
}

server.listen(port, () => {
  console.log(`Server listening on http://127.0.0.1:${port}/event`);
  console.log(`Gemini CLI working directory: ${process.cwd()}`);
});

process.on("SIGINT", cleanup);
process.on("SIGTERM", cleanup);
