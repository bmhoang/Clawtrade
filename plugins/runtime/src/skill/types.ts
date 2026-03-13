// Shared types for the Skill SDK

export interface MarketData {
  symbol: string;
  price: number;
  bid: number;
  ask: number;
  volume: number;
  timestamp: number;
}

export interface OrderRequest {
  symbol: string;
  side: "buy" | "sell";
  type: "market" | "limit" | "stop";
  quantity: number;
  price?: number;
  stopPrice?: number;
}

export interface OrderResult {
  orderId: string;
  status: "filled" | "partial" | "pending" | "rejected";
  filledQuantity: number;
  filledPrice: number;
  timestamp: number;
}
