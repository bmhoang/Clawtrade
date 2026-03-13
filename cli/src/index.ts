#!/usr/bin/env bun
// Clawtrade CLI - Entry Point

import { startRepl } from "./repl";

const API_BASE = process.env.CLAWTRADE_API || "http://127.0.0.1:9090";

async function main() {
  const args = process.argv.slice(2);
  const command = args[0];

  switch (command) {
    case "chat":
      await startRepl(API_BASE);
      break;

    case "version":
      await showVersion(API_BASE);
      break;

    case "health":
      await checkHealth(API_BASE);
      break;

    default:
      printUsage();
      break;
  }
}

async function showVersion(apiBase: string) {
  try {
    const resp = await fetch(`${apiBase}/api/v1/system/version`);
    const data = (await resp.json()) as { version: string };
    console.log(`Clawtrade ${data.version}`);
  } catch {
    console.error("Error: Cannot connect to Clawtrade server at", apiBase);
    process.exit(1);
  }
}

async function checkHealth(apiBase: string) {
  try {
    const resp = await fetch(`${apiBase}/api/v1/system/health`);
    const data = (await resp.json()) as { status: string; version: string };
    console.log(`Status: ${data.status}`);
    console.log(`Version: ${data.version}`);
  } catch {
    console.error("Error: Cannot connect to Clawtrade server at", apiBase);
    process.exit(1);
  }
}

function printUsage() {
  console.log("Clawtrade CLI");
  console.log("");
  console.log("Usage: clawtrade-cli <command>");
  console.log("");
  console.log("Commands:");
  console.log("  chat       Start interactive chat with AI trading agent");
  console.log("  version    Show server version");
  console.log("  health     Check server health");
  console.log("");
  console.log("Environment:");
  console.log("  CLAWTRADE_API  Server URL (default: http://127.0.0.1:9090)");
}

main();
