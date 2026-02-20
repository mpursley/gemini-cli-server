#!/usr/bin/env node
const http = require("http");
const fs = require("fs");
const path = require("path");
const { spawn, spawnSync } = require("child_process");

const port = Number(process.argv[2] || process.env.PORT || 8765);



function runGemini(prompt, sessionId = null) {
  return new Promise((resolve, reject) => {
    const args = ["--yolo", "--prompt", prompt, "-m", "gemini-2.5-flash", "--output-format", "json"];
    if (sessionId) {
      args.push("--resume", sessionId);
    }
    console.log(`[${new Date().toISOString()}] Executing Gemini (Session: ${sessionId || "new"})`);
    const p = spawn("gemini", args, { stdio: ["ignore", "pipe", "pipe"] });
    let out = "", err = "";
    p.stdout.on("data", d => (out += d.toString()));
    p.stderr.on("data", d => (err += d.toString()));
    p.on("close", code => {
      if (code === 0 || code === null) {
        try {
          // Find the start of the JSON object in case of warnings/logs
          const jsonStart = out.indexOf('{\n  "session_id"');
          if (jsonStart === -1) {
            resolve({ reply: out.trim(), session_id: sessionId });
          } else {
            const jsonStr = out.substring(jsonStart);
            const parsed = JSON.parse(jsonStr);
            resolve({ reply: parsed.response, session_id: parsed.session_id });
          }
        } catch (e) {
          resolve({ reply: out.trim(), session_id: sessionId });
        }
      } else {
        reject(new Error(err || `exit ${code}`));
      }
    });
  });
}

function listSessions() {
  return new Promise((resolve, reject) => {
    console.log(`[${new Date().toISOString()}] Listing sessions...`);
    const p = spawn("gemini", ["--list-sessions"], { stdio: ["ignore", "pipe", "pipe"] });
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

      if (!message) {
        res.writeHead(400, { "Content-Type": "application/json" });
        return res.end(JSON.stringify({ ok: false, error: "No message provided" }));
      }

      try {
        const result = await runGemini(message, sessionId);
        res.writeHead(200, { "Content-Type": "application/json" });
        res.end(JSON.stringify({ ok: true, reply: result.reply, sessionId: result.session_id }));
      } catch (e) {
        res.writeHead(500, { "Content-Type": "application/json" });
        res.end(JSON.stringify({ ok: false, error: String(e.message || e) }));
      }
    });
    return;
  }

  res.writeHead(404, { "Content-Type": "text/plain" });
  res.end("Not found");
});

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
