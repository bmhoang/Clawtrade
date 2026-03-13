export interface TradeNotification {
  type: 'trade_filled' | 'price_alert' | 'risk_warning' | 'ai_suggestion' | 'system';
  title: string;
  body: string;
  symbol?: string;
  urgency: 'low' | 'medium' | 'high';
}

export function showLocalNotification(notification: TradeNotification): void {
  if (Notification.permission !== 'granted') return;

  const icon = notification.urgency === 'high' ? '\u{1F6A8}' : notification.type === 'trade_filled' ? '\u2705' : '\u{1F4CA}';

  new Notification(`${icon} ${notification.title}`, {
    body: notification.body,
    tag: notification.type,
  });
}

export function createTradeFilledNotification(symbol: string, side: string, price: number): TradeNotification {
  return {
    type: 'trade_filled',
    title: `${side.toUpperCase()} ${symbol} Filled`,
    body: `Your ${side} order for ${symbol} was filled at $${price.toFixed(2)}`,
    symbol,
    urgency: 'medium',
  };
}

export function createPriceAlertNotification(symbol: string, price: number, threshold: number): TradeNotification {
  const direction = price > threshold ? 'above' : 'below';
  return {
    type: 'price_alert',
    title: `${symbol} Price Alert`,
    body: `${symbol} is now ${direction} $${threshold} (current: $${price.toFixed(2)})`,
    symbol,
    urgency: 'high',
  };
}

export function createRiskWarningNotification(message: string): TradeNotification {
  return {
    type: 'risk_warning',
    title: 'Risk Warning',
    body: message,
    urgency: 'high',
  };
}
