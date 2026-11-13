import * as sdk from "@anthropic-ai/claude-agent-sdk";
import { createInterface } from "node:readline";
import { createBridge } from "./bridge.mjs";

const bridge = createBridge(
  sdk,
  (packet) => process.stdout.write(`${JSON.stringify(packet)}\n`),
  { executable: process.argv[2], stateDirectory: process.argv[3] },
);
const input = createInterface({ input: process.stdin, crlfDelay: Infinity });
input.on("line", (line) => {
  try {
    void bridge.handle(JSON.parse(line));
  } catch {
    input.close();
  }
});
input.on("close", () => {
  void bridge.close().finally(() => process.exit(0));
});
