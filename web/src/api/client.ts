const API_BASE = '/api/v1';

export async function fetchHealth() {
  const res = await fetch(`${API_BASE}/system/health`);
  return res.json() as Promise<{ status: string; version: string }>;
}

export async function fetchVersion() {
  const res = await fetch(`${API_BASE}/system/version`);
  return res.json() as Promise<{ version: string }>;
}

export function connectWebSocket(onMessage: (data: unknown) => void) {
  const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
  const ws = new WebSocket(`${protocol}//${location.host}/ws`);

  ws.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data);
      onMessage(data);
    } catch {
      // ignore non-JSON messages
    }
  };

  return ws;
}
