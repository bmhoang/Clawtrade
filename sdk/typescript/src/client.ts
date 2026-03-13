// Clawtrade TypeScript SDK

export interface ClawtradeConfig {
  baseURL: string;
  apiKey?: string;
  timeout?: number;
}

export interface HealthResponse {
  status: string;
  version: string;
}

export interface PortfolioResponse {
  balance: number;
  unrealizedPnl: number;
  todayPnl: number;
}

export interface Position {
  symbol: string;
  side: string;
  size: number;
  entry: number;
  current: number;
  pnl: number;
}

export interface ChatResponse {
  response: string;
  model: string;
}

export interface OrderRequest {
  symbol: string;
  side: 'buy' | 'sell';
  quantity: number;
  price?: number;
  type?: 'market' | 'limit';
}

export interface OrderResponse {
  orderId: string;
  status: string;
}

export class ClawtradeClient {
  private config: ClawtradeConfig;

  constructor(config: ClawtradeConfig) {
    this.config = {
      timeout: 30000,
      ...config,
    };
  }

  // Health check
  async health(): Promise<HealthResponse> {
    return this.get('/api/health');
  }

  // Portfolio
  async getPortfolio(): Promise<PortfolioResponse> {
    return this.get('/api/portfolio');
  }

  // Positions
  async getPositions(): Promise<Position[]> {
    return this.get('/api/positions');
  }

  // Chat with AI
  async chat(message: string): Promise<ChatResponse> {
    return this.post('/api/chat', { message });
  }

  // Place order
  async placeOrder(order: OrderRequest): Promise<OrderResponse> {
    return this.post('/api/orders', order);
  }

  // Cancel order
  async cancelOrder(orderId: string): Promise<{ success: boolean }> {
    return this.delete(`/api/orders/${orderId}`);
  }

  // Get price
  async getPrice(symbol: string): Promise<{ symbol: string; price: number }> {
    return this.get(`/api/price/${symbol}`);
  }

  // Generic HTTP methods
  private async get<T>(path: string): Promise<T> {
    const url = `${this.config.baseURL}${path}`;
    const response = await fetch(url, {
      method: 'GET',
      headers: this.headers(),
      signal: AbortSignal.timeout(this.config.timeout!),
    });
    if (!response.ok) {
      const body = await response.text();
      throw new Error(`HTTP ${response.status}: ${body}`);
    }
    return response.json() as Promise<T>;
  }

  private async post<T>(path: string, body: unknown): Promise<T> {
    const url = `${this.config.baseURL}${path}`;
    const response = await fetch(url, {
      method: 'POST',
      headers: this.headers(),
      body: JSON.stringify(body),
      signal: AbortSignal.timeout(this.config.timeout!),
    });
    if (!response.ok) {
      const text = await response.text();
      throw new Error(`HTTP ${response.status}: ${text}`);
    }
    return response.json() as Promise<T>;
  }

  private async delete<T>(path: string): Promise<T> {
    const url = `${this.config.baseURL}${path}`;
    const response = await fetch(url, {
      method: 'DELETE',
      headers: this.headers(),
      signal: AbortSignal.timeout(this.config.timeout!),
    });
    if (!response.ok) {
      const text = await response.text();
      throw new Error(`HTTP ${response.status}: ${text}`);
    }
    return response.json() as Promise<T>;
  }

  private headers(): Record<string, string> {
    const h: Record<string, string> = {
      'Content-Type': 'application/json',
    };
    if (this.config.apiKey) {
      h['Authorization'] = `Bearer ${this.config.apiKey}`;
    }
    return h;
  }
}

// Factory function
export function createClient(baseURL: string, apiKey?: string): ClawtradeClient {
  return new ClawtradeClient({ baseURL, apiKey });
}
