"""Clawtrade Python SDK"""
from dataclasses import dataclass
from typing import Optional, List, Dict, Any
import json
import urllib.request
import urllib.error


@dataclass
class ClawtradeConfig:
    base_url: str
    api_key: Optional[str] = None
    timeout: int = 30


@dataclass
class HealthResponse:
    status: str
    version: str


@dataclass
class PortfolioResponse:
    balance: float
    unrealized_pnl: float
    today_pnl: float


@dataclass
class Position:
    symbol: str
    side: str
    size: float
    entry: float
    current: float
    pnl: float


@dataclass
class ChatResponse:
    response: str
    model: str


@dataclass
class OrderRequest:
    symbol: str
    side: str  # 'buy' or 'sell'
    quantity: float
    price: Optional[float] = None
    order_type: str = 'market'


@dataclass
class OrderResponse:
    order_id: str
    status: str


class ClawtradeClient:
    """Client for interacting with the Clawtrade API."""

    def __init__(self, config: ClawtradeConfig):
        self.config = config

    def health(self) -> HealthResponse:
        """Check API health status."""
        data = self._get('/api/health')
        return HealthResponse(status=data['status'], version=data.get('version', ''))

    def get_portfolio(self) -> PortfolioResponse:
        """Get current portfolio summary."""
        data = self._get('/api/portfolio')
        return PortfolioResponse(**data)

    def get_positions(self) -> List[Position]:
        """Get all open positions."""
        data = self._get('/api/positions')
        return [Position(**p) for p in data]

    def chat(self, message: str) -> ChatResponse:
        """Send a message to the AI trading assistant."""
        data = self._post('/api/chat', {'message': message})
        return ChatResponse(**data)

    def place_order(self, order: OrderRequest) -> OrderResponse:
        """Place a new order."""
        data = self._post('/api/orders', {
            'symbol': order.symbol,
            'side': order.side,
            'quantity': order.quantity,
            'price': order.price,
            'type': order.order_type,
        })
        return OrderResponse(**data)

    def cancel_order(self, order_id: str) -> Dict[str, Any]:
        """Cancel an existing order."""
        return self._delete(f'/api/orders/{order_id}')

    def get_price(self, symbol: str) -> Dict[str, Any]:
        """Get current price for a symbol."""
        return self._get(f'/api/price/{symbol}')

    def _get(self, path: str) -> Any:
        url = f"{self.config.base_url}{path}"
        req = urllib.request.Request(url, headers=self._headers())
        with urllib.request.urlopen(req, timeout=self.config.timeout) as resp:
            return json.loads(resp.read())

    def _post(self, path: str, body: dict) -> Any:
        url = f"{self.config.base_url}{path}"
        data = json.dumps(body).encode()
        req = urllib.request.Request(url, data=data, headers=self._headers(), method='POST')
        with urllib.request.urlopen(req, timeout=self.config.timeout) as resp:
            return json.loads(resp.read())

    def _delete(self, path: str) -> Any:
        url = f"{self.config.base_url}{path}"
        req = urllib.request.Request(url, headers=self._headers(), method='DELETE')
        with urllib.request.urlopen(req, timeout=self.config.timeout) as resp:
            return json.loads(resp.read())

    def _headers(self) -> dict:
        h = {'Content-Type': 'application/json'}
        if self.config.api_key:
            h['Authorization'] = f'Bearer {self.config.api_key}'
        return h


def create_client(base_url: str, api_key: str = None) -> ClawtradeClient:
    """Create a new Clawtrade client instance."""
    return ClawtradeClient(ClawtradeConfig(base_url=base_url, api_key=api_key))
