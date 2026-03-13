// Interactive chat REPL for Clawtrade

import * as readline from "readline";

export async function startRepl(apiBase: string) {
  console.log("Clawtrade Trading Agent");
  console.log("Type your message or use commands:");
  console.log("  /portfolio  - Show portfolio");
  console.log("  /price BTC  - Get price");
  console.log("  /quit       - Exit");
  console.log("");

  const rl = readline.createInterface({
    input: process.stdin,
    output: process.stdout,
    prompt: "you> ",
  });

  rl.prompt();

  rl.on("line", async (line: string) => {
    const input = line.trim();

    if (!input) {
      rl.prompt();
      return;
    }

    if (input === "/quit" || input === "/exit") {
      console.log("Goodbye!");
      rl.close();
      process.exit(0);
    }

    if (input.startsWith("/")) {
      await handleCommand(input, apiBase);
    } else {
      await handleChat(input, apiBase);
    }

    rl.prompt();
  });

  rl.on("close", () => {
    process.exit(0);
  });
}

async function handleCommand(input: string, apiBase: string) {
  const parts = input.split(" ");
  const cmd = parts[0];

  switch (cmd) {
    case "/portfolio":
      console.log("[Portfolio view - coming in Phase 2]");
      break;

    case "/price": {
      const symbol = parts[1]?.toUpperCase() || "BTC";
      console.log(`[Price for ${symbol} - coming when exchange adapter is connected]`);
      break;
    }

    case "/help":
      console.log("Commands: /portfolio, /price <symbol>, /quit, /help");
      break;

    default:
      console.log(`Unknown command: ${cmd}. Type /help for available commands.`);
  }
}

async function handleChat(message: string, apiBase: string) {
  // For now, echo back. Will wire to AI agent in Task 17.
  console.log(`agent> [AI chat coming in Task 17 - received: "${message}"]`);
}
