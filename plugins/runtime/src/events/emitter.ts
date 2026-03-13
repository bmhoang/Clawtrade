import type { IPCClient } from "../ipc";

export interface EventData {
  type: string;
  data: Record<string, unknown>;
  timestamp: number;
}

export type EventCallback = (event: EventData) => void | Promise<void>;

/**
 * SkillEventEmitter provides event pub/sub for skills via IPC bridge.
 * Supports wildcard pattern matching consistent with the Go EventBus.
 */
export class SkillEventEmitter {
  private listeners: Map<string, Set<EventCallback>> = new Map();
  private ipc: IPCClient;

  constructor(ipc: IPCClient) {
    this.ipc = ipc;

    // Register IPC method so Go core can push events to us
    this.ipc.registerMethod("event.deliver", async (params: unknown) => {
      const event = params as EventData;
      this.handleIncoming(event);
      return { ok: true };
    });
  }

  /**
   * Subscribe to events matching a pattern (supports wildcards).
   * Returns an unsubscribe function.
   */
  on(pattern: string, callback: EventCallback): () => void {
    let set = this.listeners.get(pattern);
    if (!set) {
      set = new Set();
      this.listeners.set(pattern, set);
    }
    set.add(callback);

    // Notify Go core about the subscription
    this.ipc.call("event.subscribe", { pattern }).catch(() => {
      // Subscription notification is best-effort
    });

    return () => {
      const s = this.listeners.get(pattern);
      if (s) {
        s.delete(callback);
        if (s.size === 0) {
          this.listeners.delete(pattern);
        }
      }
    };
  }

  /**
   * Subscribe to a one-time event. Automatically removes after first match.
   * Returns an unsubscribe function for early cancellation.
   */
  once(pattern: string, callback: EventCallback): () => void {
    const wrapper: EventCallback = (event) => {
      unsub();
      return callback(event);
    };
    const unsub = this.on(pattern, wrapper);
    return unsub;
  }

  /**
   * Emit an event. Sends to Go core via IPC and also notifies local listeners.
   */
  async emit(type: string, data: Record<string, unknown>): Promise<void> {
    const event: EventData = {
      type,
      data,
      timestamp: Date.now(),
    };

    // Send to Go core
    await this.ipc.call("event.publish", event);

    // Also dispatch locally
    this.handleIncoming(event);
  }

  /**
   * Handle incoming event from Go core (called by IPC).
   * Dispatches to all listeners whose pattern matches the event type.
   */
  handleIncoming(event: EventData): void {
    for (const [pattern, callbacks] of this.listeners) {
      if (matchPattern(pattern, event.type)) {
        for (const cb of callbacks) {
          try {
            cb(event);
          } catch {
            // Don't let one listener break others
          }
        }
      }
    }
  }

  /**
   * Remove all listeners.
   */
  removeAll(): void {
    this.listeners.clear();
  }
}

/**
 * matchPattern - wildcard matching consistent with Go EventBus.
 * Supports exact match, global wildcard "*", and prefix wildcard "prefix.*".
 */
export function matchPattern(pattern: string, eventType: string): boolean {
  if (pattern === "*" || pattern === eventType) {
    return true;
  }
  if (pattern.endsWith(".*")) {
    const prefix = pattern.slice(0, -2);
    return eventType.startsWith(prefix + ".");
  }
  return false;
}
