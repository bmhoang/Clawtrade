import { describe, test, expect, mock } from "bun:test";
import { SkillEventEmitter, matchPattern } from "./emitter";
import type { EventData } from "./emitter";
import type { IPCClient } from "../ipc";

/** Create a minimal mock IPCClient for testing. */
function createMockIPC(): IPCClient {
  const methods = new Map<string, (params: unknown) => Promise<unknown>>();
  return {
    registerMethod(name: string, handler: (params: unknown) => Promise<unknown>) {
      methods.set(name, handler);
    },
    call: mock(async (_method: string, _params?: unknown) => ({ jsonrpc: "2.0" as const, result: { ok: true } })),
    // Expose registered methods for testing
    _methods: methods,
  } as unknown as IPCClient;
}

describe("SkillEventEmitter", () => {
  test("on() receives matching events", () => {
    const ipc = createMockIPC();
    const emitter = new SkillEventEmitter(ipc);

    const received: EventData[] = [];
    emitter.on("trade.order.filled", (e) => {
      received.push(e);
    });

    emitter.handleIncoming({
      type: "trade.order.filled",
      data: { id: "123" },
      timestamp: Date.now(),
    });

    emitter.handleIncoming({
      type: "trade.order.placed",
      data: { id: "456" },
      timestamp: Date.now(),
    });

    expect(received.length).toBe(1);
    expect(received[0].data.id).toBe("123");
  });

  test("wildcard patterns work", () => {
    const ipc = createMockIPC();
    const emitter = new SkillEventEmitter(ipc);

    const received: EventData[] = [];
    emitter.on("market.*", (e) => {
      received.push(e);
    });

    emitter.handleIncoming({ type: "market.price.update", data: {}, timestamp: Date.now() });
    emitter.handleIncoming({ type: "market.orderbook.update", data: {}, timestamp: Date.now() });
    emitter.handleIncoming({ type: "trade.order.filled", data: {}, timestamp: Date.now() });

    expect(received.length).toBe(2);
  });

  test("once() only fires once", () => {
    const ipc = createMockIPC();
    const emitter = new SkillEventEmitter(ipc);

    let callCount = 0;
    emitter.once("risk.alert", () => {
      callCount++;
    });

    emitter.handleIncoming({ type: "risk.alert", data: {}, timestamp: Date.now() });
    emitter.handleIncoming({ type: "risk.alert", data: {}, timestamp: Date.now() });
    emitter.handleIncoming({ type: "risk.alert", data: {}, timestamp: Date.now() });

    expect(callCount).toBe(1);
  });

  test("unsubscribe (returned function) removes listener", () => {
    const ipc = createMockIPC();
    const emitter = new SkillEventEmitter(ipc);

    let callCount = 0;
    const unsub = emitter.on("system.health", () => {
      callCount++;
    });

    emitter.handleIncoming({ type: "system.health", data: {}, timestamp: Date.now() });
    expect(callCount).toBe(1);

    unsub();

    emitter.handleIncoming({ type: "system.health", data: {}, timestamp: Date.now() });
    expect(callCount).toBe(1);
  });

  test("removeAll clears everything", () => {
    const ipc = createMockIPC();
    const emitter = new SkillEventEmitter(ipc);

    let count1 = 0;
    let count2 = 0;
    emitter.on("trade.*", () => { count1++; });
    emitter.on("risk.*", () => { count2++; });

    emitter.handleIncoming({ type: "trade.order.filled", data: {}, timestamp: Date.now() });
    emitter.handleIncoming({ type: "risk.alert", data: {}, timestamp: Date.now() });
    expect(count1).toBe(1);
    expect(count2).toBe(1);

    emitter.removeAll();

    emitter.handleIncoming({ type: "trade.order.filled", data: {}, timestamp: Date.now() });
    emitter.handleIncoming({ type: "risk.alert", data: {}, timestamp: Date.now() });
    expect(count1).toBe(1);
    expect(count2).toBe(1);
  });

  test("emit sends to IPC and dispatches locally", async () => {
    const ipc = createMockIPC();
    const emitter = new SkillEventEmitter(ipc);

    const received: EventData[] = [];
    emitter.on("ai.response", (e) => {
      received.push(e);
    });

    await emitter.emit("ai.response", { text: "hello" });

    expect(received.length).toBe(1);
    expect(received[0].data.text).toBe("hello");
    expect((ipc.call as ReturnType<typeof mock>).mock.calls.length).toBeGreaterThanOrEqual(1);
  });
});

describe("matchPattern", () => {
  test("exact match", () => {
    expect(matchPattern("trade.order.filled", "trade.order.filled")).toBe(true);
  });

  test("no match on different type", () => {
    expect(matchPattern("trade.order.filled", "trade.order.placed")).toBe(false);
  });

  test("global wildcard matches everything", () => {
    expect(matchPattern("*", "anything.at.all")).toBe(true);
  });

  test("prefix wildcard matches children", () => {
    expect(matchPattern("market.*", "market.price.update")).toBe(true);
    expect(matchPattern("market.*", "market.orderbook.update")).toBe(true);
  });

  test("prefix wildcard does not match unrelated", () => {
    expect(matchPattern("market.*", "trade.order.filled")).toBe(false);
  });

  test("prefix wildcard does not match prefix itself", () => {
    expect(matchPattern("market.*", "market")).toBe(false);
  });
});
