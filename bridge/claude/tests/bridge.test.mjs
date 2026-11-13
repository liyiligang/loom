import test from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, readFile, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { createBridge } from "../src/bridge.mjs";

const tick = () => new Promise((resolve) => setImmediate(resolve));
async function eventually(check) {
  for (let i = 0; i < 1000; i++) {
    if (check()) return;
    await new Promise((resolve) => setTimeout(resolve, 2));
  }
  assert.fail("Bridge did not reach the expected state");
}
function harness(sdk, options = {}) {
  const packets = [];
  const bridge = createBridge(
    sdk,
    (packet) => packets.push(structuredClone(packet)),
    { executable: "/installed/claude", ...options },
  );
  let sequence = 0;
  async function call(method, params = {}) {
    const id = ++sequence;
    await bridge.handle({ id, method, params });
    const response = packets.find((packet) => packet.id === id);
    if (response.error) throw new Error(response.error.message);
    return response.result;
  }
  return {
    ...bridge,
    packets,
    call,
    events: () =>
      packets
        .filter((packet) => packet.method === "event")
        .map((packet) => packet.params),
  };
}
const success = { type: "result", subtype: "success", is_error: false };

test("model changes and default resets preserve native history across restart", async () => {
  const stateDirectory = await mkdtemp(path.join(tmpdir(), "loom-models-"));
  const seen = [];
  const sdk = {
    query({ options }) {
      seen.push(options);
      return {
        close() {},
        async *[Symbol.asyncIterator]() {
          yield { type: "system", subtype: "init", session_id: options.sessionId ?? options.resume };
          yield success;
        },
      };
    },
  };
  let h = harness(sdk, { stateDirectory });
  try {
    const session = await h.call("session/create", { cwd: "/project", model: "sonnet" });
    async function run(model) {
      const turn = await h.call("turn/start", { id: session.id, text: "Model fixture", ...(model === undefined ? {} : { model }) });
      await eventually(() => h.events().some(e => e.kind === "turn.completed" && e.turnId === turn.id));
    }
    await run(undefined);
    await run("opus");
    assert.equal(seen[0].model, "sonnet");
    assert.equal(seen[1].model, "opus");
    assert.equal(seen[1].resume, session.id);
    await h.close();
    h = harness(sdk, { stateDirectory });
    await h.call("session/synchronize", { id: session.id, subscribed: false });
    await run("");
    assert.equal(seen[2].model, "default");
    assert.equal(seen[2].resume, session.id);
    const restored = await h.call("session/synchronize", { id: session.id, subscribed: true });
    assert.equal(restored.model, "");
    assert.deepEqual(restored.turns.map(turn => turn.model), ["sonnet", "opus", ""]);
  } finally {
    await h.close();
    await rm(stateDirectory, { recursive: true, force: true });
  }
});

test("full access is captured per query and switching back restores native approvals", async () => {
  const options = [];
  const h = harness({
    query(input) {
      options.push(input.options);
      return {
        close() {},
        async *[Symbol.asyncIterator]() {
          yield { type: "system", subtype: "init", session_id: input.options.sessionId ?? input.options.resume };
          yield success;
        },
      };
    },
  });
  const session = await h.call("session/create", { cwd: "/project" });
  const first = await h.call("turn/start", { id: session.id, text: "Full access", accessMode: "full" });
  await eventually(() => h.events().some(e => e.kind === "turn.completed" && e.turnId === first.id));
  assert.equal(options[0].permissionMode, "bypassPermissions");
  assert.equal(options[0].allowDangerouslySkipPermissions, true);
  const second = await h.call("turn/start", { id: session.id, text: "Restricted access", accessMode: "workspace" });
  await eventually(() => h.events().some(e => e.kind === "turn.completed" && e.turnId === second.id));
  assert.equal(options[1].permissionMode, "default");
  assert.equal(options[1].allowDangerouslySkipPermissions, undefined);
  assert.equal(options[1].resume, session.id);
  const state = await h.call("session/synchronize", { id: session.id, subscribed: true });
  assert.deepEqual(state.turns.map(turn => turn.accessMode), ["full", "workspace"]);
  await assert.rejects(h.call("turn/start", { id: session.id, text: "Invalid access", accessMode: "invalid" }), /Choose workspace access/);
  assert.equal(options.length, 2);
  await h.close();
});

// No real model, credentials, or agent process is used by these SDK fixtures.
test("confirmation operations create empty sessions without starting an SDK query", async () => {
  const h = harness({
    query() {
      assert.fail("Unexpected model task");
    },
  });
  await h.call("initialize");
  const session = await h.call("session/create", { cwd: "/project" });
  assert.equal(session.turns.length, 0);
  assert.deepEqual(
    await h.call("session/synchronize", { id: session.id, subscribed: true }),
    session,
  );
  await h.close();
});

test("task acknowledgement precedes streaming and multiple blocks produce one final message", async () => {
  let options;
  const h = harness({
    query(input) {
      options = input.options;
      return {
        close() {},
        async *[Symbol.asyncIterator]() {
          yield {
            type: "system",
            subtype: "init",
            session_id: input.options.sessionId,
          };
          yield { type: "stream_event", event: { type: "message_start" } };
          for (const [index, text] of ["Hello", "World"].entries()) {
            yield {
              type: "stream_event",
              event: {
                type: "content_block_start",
                index,
                content_block: { type: "text", text: "" },
              },
            };
            yield {
              type: "stream_event",
              event: {
                type: "content_block_delta",
                index,
                delta: { type: "text_delta", text },
              },
            };
          }
          yield {
            type: "assistant",
            uuid: "native-message",
            message: {
              content: [
                { type: "text", text: "Hello" },
                { type: "text", text: "World" },
              ],
            },
          };
          yield success;
        },
      };
    },
  });
  const session = await h.call("session/create", { cwd: "/project" });
  const turn = await h.call("turn/start", { id: session.id, text: "Hi" });
  assert.equal(h.events().length, 0);
  await eventually(() =>
    h.events().some((event) => event.kind === "turn.completed"),
  );
  const completed = h
    .events()
    .find((event) => event.kind === "turn.completed").turn;
  assert.equal(completed.state, "completed");
  const messages = completed.items.filter((item) => item.role === "assistant");
  assert.equal(messages.length, 1);
  assert.equal(messages[0].text, "Hello\nWorld");
  const deltas = h.events().filter((event) => event.kind === "message.delta");
  assert.equal(deltas.map((event) => event.delta).join(""), messages[0].text);
  assert.equal(deltas[0].turnId, turn.id);
  assert.equal(options.cwd, "/project");
  assert.equal(options.pathToClaudeCodeExecutable, "/installed/claude");
  assert.equal(options.permissionMode, "default");
  assert.deepEqual(options.settingSources, ["user", "project", "local"]);
  await h.close();
});

test("tool approvals and questions wait for replies and retain command output", async () => {
  const decisions = [];
  const h = harness({
    query({ options }) {
      return {
        close() {},
        async *[Symbol.asyncIterator]() {
          yield {
            type: "system",
            subtype: "init",
            session_id: options.sessionId,
          };
          yield {
            type: "assistant",
            message: {
              content: [
                {
                  type: "tool_use",
                  id: "tool-1",
                  name: "Bash",
                  input: { command: "git status" },
                },
              ],
            },
          };
          decisions.push(
            await options.canUseTool("Bash", { command: "git status" }, {}),
          );
          decisions.push(
            await options.canUseTool(
              "AskUserQuestion",
              {
                questions: [
                  {
                    header: "Choice",
                    question: "Which branch?",
                    options: [{ label: "Main", description: "Use main" }],
                  },
                ],
              },
              {},
            ),
          );
          yield {
            type: "user",
            message: {
              content: [
                {
                  type: "tool_result",
                  tool_use_id: "tool-1",
                  content: "clean",
                },
              ],
            },
          };
          yield success;
        },
      };
    },
  });
  const session = await h.call("session/create", { cwd: "/project" });
  await h.call("turn/start", { id: session.id, text: "Inspect" });
  await eventually(() =>
    h.events().some((event) => event.kind === "approval.requested"),
  );
  assert.equal(decisions.length, 0);
  const approval = h
    .events()
    .find((event) => event.kind === "approval.requested");
  await h.handle({ id: approval.requestId, result: { decision: "accept" } });
  await eventually(() =>
    h.events().some((event) => event.kind === "question.requested"),
  );
  const question = h
    .events()
    .find((event) => event.kind === "question.requested");
  await h.handle({
    id: question.requestId,
    result: { answers: { 0: { answers: ["Main"] } } },
  });
  await eventually(() =>
    h.events().some((event) => event.kind === "turn.completed"),
  );
  assert.equal(decisions[0].behavior, "allow");
  assert.deepEqual(decisions[1].updatedInput.answers, {
    "Which branch?": "Main",
  });
  assert.equal(
    h.events().filter((event) => event.kind === "interaction.resolved").length,
    2,
  );
  const item = h
    .events()
    .find((event) => event.kind === "turn.completed")
    .turn.items.find((item) => item.id === "tool-1");
  assert.equal(item.detail.command, "git status");
  assert.equal(item.detail.type, "commandExecution");
  assert.equal(item.output, "clean");
  await h.close();
});

test("interruption acknowledgement waits for iterator completion and cancels a pending approval", async () => {
  let abort;
  const h = harness({
    query({ options }) {
      const signal = new AbortController();
      abort = () => signal.abort();
      return {
        close: abort,
        async interrupt() {
          abort();
        },
        async *[Symbol.asyncIterator]() {
          await options.canUseTool(
            "Bash",
            { command: "sleep 1" },
            { signal: signal.signal },
          );
          await tick();
        },
      };
    },
  });
  const session = await h.call("session/create", { cwd: "/project" });
  const turn = await h.call("turn/start", { id: session.id, text: "Wait" });
  await eventually(() =>
    h.events().some((event) => event.kind === "approval.requested"),
  );
  await h.call("turn/interrupt", { id: session.id, turnId: turn.id });
  await eventually(() =>
    h.events().some((event) => event.kind === "turn.completed"),
  );
  assert.equal(
    h.events().find((event) => event.kind === "turn.completed").turn.state,
    "interrupted",
  );
  assert.ok(h.events().some((event) => event.kind === "interaction.resolved"));
  await h.close();
});

test("completed history resumes while interrupted checkpoints remain unknown and never resubmit", async (t) => {
  const directory = await mkdtemp(path.join(tmpdir(), "loom-bridge-"));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const calls = [];
  const sdk = {
    query({ options }) {
      calls.push(options);
      return {
        close() {},
        async *[Symbol.asyncIterator]() {
          yield {
            type: "system",
            subtype: "init",
            session_id: options.sessionId ?? options.resume,
          };
          yield success;
        },
      };
    },
  };
  const first = harness(sdk, { stateDirectory: directory });
  const session = await first.call("session/create", { cwd: "/project" });
  await first.call("turn/start", { id: session.id, text: "First" });
  await eventually(() =>
    first.events().some((event) => event.kind === "turn.completed"),
  );
  await first.close();
  const second = harness(sdk, { stateDirectory: directory });
  const restored = await second.call("session/synchronize", { id: session.id });
  assert.equal(restored.turns[0].state, "completed");
  await second.call("turn/start", { id: session.id, text: "Next" });
  await eventually(() =>
    second.events().some((event) => event.kind === "turn.completed"),
  );
  assert.equal(calls[1].resume, session.id);
  await second.close();
  const checkpoint = JSON.parse(
    await readFile(path.join(directory, `${session.id}.json`), "utf8"),
  );
  checkpoint.turns.at(-1).state = "running";
  await writeFile(
    path.join(directory, `${session.id}.json`),
    JSON.stringify(checkpoint),
  );
  const third = harness(sdk, { stateDirectory: directory });
  assert.equal(
    (await third.call("session/synchronize", { id: session.id })).turns.at(-1)
      .state,
    "unknown",
  );
  await assert.rejects(
    third.call("turn/start", { id: session.id, text: "Duplicate?" }),
    /Wait for the active task/,
  );
  assert.equal(calls.length, 2);
  await third.close();
});

test("unexpected SDK EOF blocks the session while other sessions run independently", async () => {
  const h = harness({
    query({ prompt }) {
      return {
        close() {},
        async *[Symbol.asyncIterator]() {
          if (prompt === "Good") yield success;
        },
      };
    },
  });
  const a = await h.call("session/create", { cwd: "/a" }),
    b = await h.call("session/create", { cwd: "/b" });
  await Promise.all([
    h.call("turn/start", { id: a.id, text: "Unknown" }),
    h.call("turn/start", { id: b.id, text: "Good" }),
  ]);
  await eventually(
    () =>
      h.events().filter((event) => event.kind === "turn.completed").length ===
      2,
  );
  assert.equal(
    h
      .events()
      .find(
        (event) => event.kind === "turn.completed" && event.sessionId === a.id,
      ).turn.state,
    "unknown",
  );
  assert.equal(
    h
      .events()
      .find(
        (event) => event.kind === "turn.completed" && event.sessionId === b.id,
      ).turn.state,
    "completed",
  );
  await assert.rejects(
    h.call("turn/start", { id: a.id, text: "Retry" }),
    /Wait for the active task/,
  );
  await h.close();
});

test("project credential MCP capabilities stay per query and out of saved history", async () => {
  const directory = await mkdtemp(path.join(tmpdir(), "loom-project-credentials-"));
  const seen = [];
  const h = harness({ query(input) {
    seen.push(input.options);
    return { close() {}, async *[Symbol.asyncIterator]() {
      yield { type: "system", subtype: "init", session_id: input.options.sessionId ?? input.options.resume };
      yield success;
    }};
  }}, { stateDirectory: directory });
  try {
    const first = await h.call("session/create", { cwd: "/first" });
    const second = await h.call("session/create", { cwd: "/second" });
    const tools = { url: "http://127.0.0.1:12345/mcp", token: "dummy-session-capability" };
    await h.call("turn/start", { id: first.id, text: "Use project API", credentialTools: tools });
    await eventually(() => seen.length === 1 && h.events().some(value => value.kind === "turn.completed"));
    await h.call("turn/start", { id: second.id, text: "Independent project" });
    await eventually(() => seen.length === 2 && h.events().filter(value => value.kind === "turn.completed").length === 2);
    assert.equal(seen[0].mcpServers.loom_credentials.headers.Authorization, `Bearer ${tools.token}`);
    assert.equal(seen[0].mcpServers.loom_credentials.type, "http");
    assert.equal(seen[1].mcpServers, undefined);
    assert.ok(!JSON.stringify(seen[0].env).includes(tools.token));
    assert.ok(!(await readFile(path.join(directory, `${first.id}.json`), "utf8")).includes(tools.token));
    assert.ok(!JSON.stringify(h.events()).includes(tools.token));
  } finally { await h.close(); await rm(directory, { recursive: true, force: true }); }
});
