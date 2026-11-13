import { randomUUID } from "node:crypto";
import { mkdir, readFile, writeFile, rename } from "node:fs/promises";
import path from "node:path";

const bounded = (text) =>
  text.length > 1_048_576 ? text.slice(0, 1_048_576) : text;
const terminal = (state) =>
  ["completed", "failed", "interrupted"].includes(state);
const textOf = (content) =>
  typeof content === "string"
    ? content
    : (content ?? [])
        .filter((block) => block.type === "text")
        .map((block) => block.text)
        .join("\n");

// The bridge presents the same session/turn/interaction operations as the Go adapters.
export function createBridge(sdk, emit, { executable, stateDirectory } = {}) {
  const sessions = new Map(),
    active = new Map(),
    pending = new Map();
  let stopping = false;
  const event = (value) => emit({ method: "event", params: value });
  const statePath = (id) => path.join(stateDirectory, `${id}.json`);
  function validID(id) {
    if (!/^[a-f0-9-]{36}$/.test(id ?? ""))
      throw new Error("The local session identifier is invalid.");
  }
  async function persist(session) {
    if (!stateDirectory) return;
    await mkdir(stateDirectory, { recursive: true, mode: 0o700 });
    const target = statePath(session.id),
      temporary = `${target}.${randomUUID()}.tmp`;
    await writeFile(temporary, JSON.stringify(session), { mode: 0o600 });
    await rename(temporary, target);
  }
  async function getSession(id) {
    validID(id);
    if (!sessions.has(id)) {
      if (!stateDirectory)
        throw new Error("Local session history is unavailable.");
      let session;
      try {
        session = JSON.parse(await readFile(statePath(id), "utf8"));
      } catch {
        throw new Error("Local session history is unavailable.");
      }
      if (session.id !== id)
        throw new Error("The local session identifier is invalid.");
      for (const turn of session.turns)
        if (!terminal(turn.state)) turn.state = "unknown";
      sessions.set(id, session);
    }
    return sessions.get(id);
  }
  function snapshot(session) {
    return { id: session.id, model: session.model ?? "", turns: session.turns };
  }
  function base(session, turn) {
    return { sessionId: session.id, turnId: turn.id };
  }
  function itemEvent(session, turn, item, kind = "item.completed") {
    event({ ...base(session, turn), kind, item });
  }
  function messageItems(message, turn, status = "completed") {
    const items = [];
    const content = message.message?.content;
    const text = textOf(content);
    if (text)
      items.push({
        id: message.uuid ?? randomUUID(),
        kind: "message",
        role: message.type,
        text: bounded(text),
        status,
      });
    for (const block of Array.isArray(content) ? content : []) {
      if (block.type === "tool_use")
        items.push({
          id: block.id,
          kind: "activity",
          text: "",
          status,
          detail: {
            type:
              block.name === "Bash"
                ? "commandExecution"
                : ["Edit", "Write", "MultiEdit"].includes(block.name)
                  ? "fileChange"
                  : "tool",
            tool: block.name,
            command: block.input?.command,
            input: block.input,
            status,
          },
        });
      if (block.type === "tool_result")
        items.push({
          id: block.tool_use_id,
          kind: "activity",
          output: bounded(textOf(block.content)),
          text: "",
          status: block.is_error ? "failed" : "completed",
          detail: {
            type: "tool",
            status: block.is_error ? "failed" : "completed",
          },
        });
    }
    return items;
  }
  function upsert(turn, item) {
    const old = turn.items.find((value) => value.id === item.id);
    if (old) {
      if (item.detail)
        item.detail = {
          ...old.detail,
          ...item.detail,
          type: old.detail?.type ?? item.detail.type,
        };
      Object.assign(old, item);
    } else turn.items.push(item);
  }
  async function permission(session, turn, name, input, options) {
    const id = randomUUID(),
      question = name === "AskUserQuestion";
    const questions = (input.questions ?? []).map((value, index) => ({
      id: String(index),
      header: value.header,
      question: value.question,
      options: value.options ?? [],
    }));
    const payload = question
      ? { questions }
      : {
          tool: name,
          reason: "The tool requests permission.",
          command: input.command,
          grantRoot: input.file_path,
        };
    event({
      ...base(session, turn),
      requestId: id,
      kind: question ? "question.requested" : "approval.requested",
      interactionKind: question ? "question" : "approval",
      payload,
    });
    return await new Promise((resolve) => {
      const finish = (answer) => {
        if (!pending.delete(id)) return;
        options?.signal?.removeEventListener("abort", abort);
        event({
          ...base(session, turn),
          kind: "interaction.resolved",
          resolvedId: id,
        });
        if (question && answer?.answers) {
          const answers = Object.fromEntries(
            questions.map((value) => [
              value.question,
              answer.answers[value.id]?.answers?.join("\n") ?? "",
            ]),
          );
          resolve({ behavior: "allow", updatedInput: { ...input, answers } });
        } else if (answer?.decision === "accept")
          resolve({ behavior: "allow", updatedInput: input });
        else
          resolve({
            behavior: "deny",
            message: "The user declined this request.",
          });
      };
      const abort = () => finish(undefined);
      pending.set(id, { finish, sessionId: session.id });
      options?.signal?.addEventListener("abort", abort, { once: true });
      if (options?.signal?.aborted) abort();
    });
  }
  async function run(session, turn, text, control) {
    let failure = "",
      completed = false;
    try {
      const env = { ...process.env, CLAUDE_CODE_STARTUP_FAILURE_RESULTS: "1" };
      delete env.CLAUDECODE;
      const query = sdk.query({
        prompt: text,
        options: {
          cwd: session.cwd,
          pathToClaudeCodeExecutable: executable,
          env,
          permissionMode: control.accessMode === "full" ? "bypassPermissions" : "default",
          ...(control.accessMode === "full" ? { allowDangerouslySkipPermissions: true } : {}),
          settingSources: ["user", "project", "local"],
          includePartialMessages: true,
          ...(control.credentialTools ? { mcpServers: { loom_credentials: { type: "http", url: control.credentialTools.url, headers: { Authorization: `Bearer ${control.credentialTools.token}` }, alwaysLoad: true } } } : {}),
          ...(control.model ? { model: control.model } : control.resetModel ? { model: "default" } : {}),
          ...(session.started
            ? { resume: session.id }
            : { sessionId: session.id }),
          canUseTool: (name, input, options) =>
            permission(session, turn, name, input, options),
        },
      });
      control.query = query;
      let streamed;
      const blocks = new Map();
      for await (const message of query) {
        if (message.type === "system" && message.subtype === "init") {
          if (message.session_id !== session.id)
            throw new Error(
              "The agent returned a mismatched session identifier",
            );
          session.started = true;
          await persist(session);
        }
        if (message.type === "stream_event") {
          const raw = message.event;
          if (raw.type === "message_start") {
            streamed = undefined;
            blocks.clear();
          }
          if (
            raw.type === "content_block_start" &&
            raw.content_block?.type === "text"
          ) {
            blocks.set(raw.index, raw.content_block.text ?? "");
            streamed ??= {
              id: randomUUID(),
              kind: "message",
              role: "assistant",
              text: "",
              status: "running",
            };
            upsert(turn, streamed);
            event({
              ...base(session, turn),
              kind: "item.started",
              item: streamed,
            });
          }
          if (
            raw.type === "content_block_delta" &&
            raw.delta?.type === "text_delta"
          ) {
            if (streamed && blocks.has(raw.index)) {
              const previous = streamed.text;
              blocks.set(
                raw.index,
                bounded(blocks.get(raw.index) + raw.delta.text),
              );
              streamed.text = bounded([...blocks.values()].join("\n"));
              upsert(turn, streamed);
              if (streamed.text.startsWith(previous))
                event({
                  ...base(session, turn),
                  kind: "message.delta",
                  itemId: streamed.id,
                  delta: streamed.text.slice(previous.length),
                });
            }
          }
        }
        if (message.type === "assistant") {
          const items = messageItems(message, turn);
          for (const item of items) {
            if (item.kind === "message" && streamed) {
              item.id = streamed.id;
              streamed = undefined;
              blocks.clear();
            }
            upsert(turn, item);
            itemEvent(
              session,
              turn,
              item,
              item.kind === "activity" ? "item.started" : "item.completed",
            );
          }
        }
        if (
          message.type === "user" &&
          Array.isArray(message.message?.content)
        ) {
          for (const item of messageItems(message, turn)) {
            if (item.kind === "activity") {
              upsert(turn, item);
              itemEvent(session, turn, item);
            }
          }
        }
        if (message.type === "result") {
          completed = true;
          if (message.is_error || message.subtype !== "success")
            failure =
              (message.errors ?? []).join("\n") ||
              message.result ||
              "The local agent task failed.";
        }
      }
      if (!completed && !control.interrupt)
        failure =
          "The agent has not confirmed this task's outcome. Further submission is blocked to avoid duplicate execution.";
    } catch (cause) {
      failure = cause instanceof Error ? cause.message : String(cause);
    } finally {
      const interrupted = control.interrupt ? await control.interrupt : false;
      turn.state = interrupted
        ? "interrupted"
        : failure
          ? completed
            ? "failed"
            : "unknown"
          : "completed";
      turn.error = failure;
      for (const request of [...pending.values()])
        if (request.sessionId === session.id) request.finish(undefined);
      try {
        await persist(session);
      } catch (cause) {
        turn.state = "unknown";
        turn.error = "Could not save local agent history.";
      }
      event({
        ...base(session, turn),
        kind: "turn.completed",
        turn: { ...turn },
      });
      try {
        control.query?.close();
      } catch {}
      active.delete(session.id);
    }
  }
  async function request(packet) {
    if (packet.method === "initialize")
      return { version: "Claude Agent SDK 0.3.318" };
    if (packet.method === "session/create") {
      const session = {
        id: randomUUID(),
        cwd: packet.params.cwd,
        model: packet.params.model,
        started: false,
        turns: [],
      };
      sessions.set(session.id, session);
      await persist(session);
      return snapshot(session);
    }
    if (packet.method === "session/synchronize") {
      const session = await getSession(packet.params.id);
      if (!session.started && !packet.params.subscribed)
        throw new Error(
          "The agent cannot restore an empty session after its connection closes. Create a new session.",
        );
      return snapshot(session);
    }
    if (packet.method === "turn/start") {
      if (stopping) throw new Error("agent connection is closed");
      const accessMode = packet.params.accessMode ?? "workspace";
      if (!["workspace", "full"].includes(accessMode))
        throw new Error("Choose workspace access or full access.");
      const session = await getSession(packet.params.id);
      if (
        active.has(session.id) ||
        session.turns.some((turn) => turn.state === "unknown")
      )
        throw new Error(
          "Wait for the active task or synchronize the session before sending another task.",
        );
      const turn = {
        id: randomUUID(),
        accessMode,
        model: packet.params.model ?? session.model ?? "",
        state: "running",
        items: [
          {
            id: randomUUID(),
            kind: "message",
            role: "user",
            text: packet.params.text,
            status: "completed",
          },
        ],
      };
      const control = { query: undefined, interrupt: undefined, credentialTools: packet.params.credentialTools, accessMode, model: turn.model, resetModel: packet.params.model === "" };
      active.set(session.id, control);
      const previousModel = session.model;
      session.model = turn.model;
      session.turns.push(turn);
      try {
        await persist(session);
      } catch (cause) {
        session.turns.pop();
        session.model = previousModel;
        active.delete(session.id);
        throw cause;
      }
      // setImmediate runs after handle writes the request acknowledgement.
      control.completion = new Promise((resolve) =>
        setImmediate(async () => {
          if (stopping) {
            turn.state = "unknown";
            await persist(session).catch(() => {});
            active.delete(session.id);
            resolve();
            return;
          }
          event({
            ...base(session, turn),
            kind: "turn.started",
            turn: { id: turn.id, state: "running" },
          });
          itemEvent(session, turn, turn.items[0]);
          await run(session, turn, packet.params.text, control);
          resolve();
        }),
      );
      return { id: turn.id, state: "running" };
    }
    if (packet.method === "turn/interrupt") {
      const control = active.get(packet.params.id);
      const session = await getSession(packet.params.id);
      if (!control?.query || session.turns.at(-1)?.id !== packet.params.turnId)
        throw new Error("Task not found.");
      control.interrupt = control.query.interrupt().then(
        () => true,
        () => false,
      );
      if (!(await control.interrupt))
        throw new Error("Could not interrupt the local agent.");
      return { ok: true };
    }
    throw new Error("The requested item was not found.");
  }
  async function handle(packet) {
    if (!packet.method) {
      pending.get(packet.id)?.finish(packet.result);
      return;
    }
    try {
      const result = await request(packet);
      if (packet.id !== undefined) emit({ id: packet.id, result });
    } catch (cause) {
      if (packet.id !== undefined)
        emit({
          id: packet.id,
          error: {
            code: -32000,
            message: cause instanceof Error ? cause.message : String(cause),
          },
        });
    }
  }
  async function close() {
    stopping = true;
    for (const request of [...pending.values()]) request.finish(undefined);
    for (const control of active.values()) {
      try {
        control.query?.close();
      } catch {}
    }
    await Promise.allSettled(
      [...active.values()].map((value) => value.completion),
    );
  }
  return { handle, close };
}
