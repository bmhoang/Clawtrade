#!/usr/bin/env python3
"""MT5 Bridge — connects Clawtrade (Go) to MetaTrader 5 via Python API.

Protocol: reads JSON commands from stdin, writes JSON responses to stdout.
One JSON object per line (JSONL).

Usage:
    python scripts/mt5_bridge.py [--terminal PATH_TO_TERMINAL]
"""

import sys
import json
import argparse
import subprocess
from datetime import datetime

try:
    import MetaTrader5 as mt5
except ImportError:
    # Auto-install MetaTrader5 package
    print(json.dumps({"id": "auto_install", "result": {"status": "installing MetaTrader5 package..."}}), flush=True)
    try:
        subprocess.check_call([sys.executable, "-m", "pip", "install", "MetaTrader5", "-q"],
                              stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        import MetaTrader5 as mt5
    except Exception as e:
        print(json.dumps({"error": f"Failed to install MetaTrader5: {e}"}), flush=True)
        sys.exit(1)

# Timeframe mapping
TIMEFRAMES = {
    "1m":  mt5.TIMEFRAME_M1,
    "5m":  mt5.TIMEFRAME_M5,
    "15m": mt5.TIMEFRAME_M15,
    "30m": mt5.TIMEFRAME_M30,
    "1h":  mt5.TIMEFRAME_H1,
    "4h":  mt5.TIMEFRAME_H4,
    "1d":  mt5.TIMEFRAME_D1,
    "1w":  mt5.TIMEFRAME_W1,
}

ORDER_TYPES = {
    "market_buy":  mt5.ORDER_TYPE_BUY,
    "market_sell": mt5.ORDER_TYPE_SELL,
    "limit_buy":   mt5.ORDER_TYPE_BUY_LIMIT,
    "limit_sell":  mt5.ORDER_TYPE_SELL_LIMIT,
    "stop_buy":    mt5.ORDER_TYPE_BUY_STOP,
    "stop_sell":   mt5.ORDER_TYPE_SELL_STOP,
}


def respond(data):
    print(json.dumps(data, default=str), flush=True)


def handle_init(params):
    kwargs = {}
    if params.get("terminal"):
        kwargs["path"] = params["terminal"]
    if params.get("login"):
        kwargs["login"] = int(params["login"])
    if params.get("password"):
        kwargs["password"] = params["password"]
    if params.get("server"):
        kwargs["server"] = params["server"]

    if not mt5.initialize(**kwargs):
        error = mt5.last_error()
        return {"ok": False, "error": f"MT5 init failed: {error}"}

    info = mt5.terminal_info()
    account = mt5.account_info()
    return {
        "ok": True,
        "terminal": {
            "name": info.name if info else "unknown",
            "build": info.build if info else 0,
            "connected": info.connected if info else False,
        },
        "account": {
            "login": account.login if account else 0,
            "server": account.server if account else "",
            "balance": account.balance if account else 0,
            "currency": account.currency if account else "",
            "company": account.company if account else "",
        } if account else None,
    }


def handle_shutdown(_params):
    mt5.shutdown()
    return {"ok": True}


def handle_get_price(params):
    symbol = params["symbol"]
    tick = mt5.symbol_info_tick(symbol)
    if tick is None:
        # Try enabling the symbol first
        mt5.symbol_select(symbol, True)
        tick = mt5.symbol_info_tick(symbol)
        if tick is None:
            return {"error": f"No tick data for {symbol}"}

    return {
        "symbol": symbol,
        "bid": tick.bid,
        "ask": tick.ask,
        "last": tick.last,
        "volume": tick.volume,
        "time": int(tick.time),
    }


def handle_get_candles(params):
    symbol = params["symbol"]
    tf_str = params.get("timeframe", "1h")
    limit = params.get("limit", 100)

    tf = TIMEFRAMES.get(tf_str)
    if tf is None:
        return {"error": f"Unknown timeframe: {tf_str}"}

    mt5.symbol_select(symbol, True)
    rates = mt5.copy_rates_from_pos(symbol, tf, 0, limit)
    if rates is None or len(rates) == 0:
        return {"error": f"No candle data for {symbol}", "candles": []}

    candles = []
    for r in rates:
        candles.append({
            "time": int(r["time"]),
            "open": float(r["open"]),
            "high": float(r["high"]),
            "low": float(r["low"]),
            "close": float(r["close"]),
            "volume": float(r["tick_volume"]),
        })
    return {"candles": candles}


def handle_get_balances(_params):
    account = mt5.account_info()
    if account is None:
        return {"error": "Cannot get account info"}

    return {
        "balances": [{
            "asset": account.currency,
            "free": account.margin_free,
            "locked": account.margin,
            "total": account.balance,
        }],
        "equity": account.equity,
        "margin": account.margin,
        "free_margin": account.margin_free,
        "margin_level": account.margin_level,
        "profit": account.profit,
    }


def handle_get_positions(_params):
    positions = mt5.positions_get()
    if positions is None:
        return {"positions": []}

    result = []
    for p in positions:
        result.append({
            "ticket": p.ticket,
            "symbol": p.symbol,
            "side": "buy" if p.type == mt5.POSITION_TYPE_BUY else "sell",
            "size": p.volume,
            "entry_price": p.price_open,
            "current_price": p.price_current,
            "pnl": p.profit,
            "swap": p.swap,
            "commission": p.commission,
            "opened_at": int(p.time),
            "comment": p.comment,
        })
    return {"positions": result}


def handle_get_open_orders(_params):
    orders = mt5.orders_get()
    if orders is None:
        return {"orders": []}

    result = []
    for o in orders:
        side = "buy" if o.type in (mt5.ORDER_TYPE_BUY, mt5.ORDER_TYPE_BUY_LIMIT, mt5.ORDER_TYPE_BUY_STOP) else "sell"
        order_type = "market"
        if o.type in (mt5.ORDER_TYPE_BUY_LIMIT, mt5.ORDER_TYPE_SELL_LIMIT):
            order_type = "limit"
        elif o.type in (mt5.ORDER_TYPE_BUY_STOP, mt5.ORDER_TYPE_SELL_STOP):
            order_type = "stop"

        result.append({
            "id": str(o.ticket),
            "symbol": o.symbol,
            "side": side,
            "type": order_type,
            "price": o.price_open,
            "size": o.volume_current,
            "created_at": int(o.time_setup),
            "comment": o.comment,
        })
    return {"orders": result}


def handle_place_order(params):
    symbol = params["symbol"]
    side = params["side"]  # "buy" or "sell"
    size = float(params["size"])
    order_type = params.get("type", "market")
    price = params.get("price", 0.0)
    sl = params.get("stop_loss", 0.0)
    tp = params.get("take_profit", 0.0)
    comment = params.get("comment", "clawtrade")

    info = mt5.symbol_info(symbol)
    if info is None:
        mt5.symbol_select(symbol, True)
        info = mt5.symbol_info(symbol)
        if info is None:
            return {"error": f"Symbol {symbol} not found"}

    # Determine order type
    if order_type == "market":
        mt5_type = mt5.ORDER_TYPE_BUY if side == "buy" else mt5.ORDER_TYPE_SELL
        fill_price = info.ask if side == "buy" else info.bid
    elif order_type == "limit":
        mt5_type = mt5.ORDER_TYPE_BUY_LIMIT if side == "buy" else mt5.ORDER_TYPE_SELL_LIMIT
        fill_price = float(price)
    elif order_type == "stop":
        mt5_type = mt5.ORDER_TYPE_BUY_STOP if side == "buy" else mt5.ORDER_TYPE_SELL_STOP
        fill_price = float(price)
    else:
        return {"error": f"Unknown order type: {order_type}"}

    request = {
        "action": mt5.TRADE_ACTION_DEAL if order_type == "market" else mt5.TRADE_ACTION_PENDING,
        "symbol": symbol,
        "volume": size,
        "type": mt5_type,
        "price": fill_price,
        "deviation": 20,
        "magic": 123456,
        "comment": comment,
        "type_time": mt5.ORDER_TIME_GTC,
        "type_filling": mt5.ORDER_FILLING_IOC,
    }
    if sl:
        request["sl"] = float(sl)
    if tp:
        request["tp"] = float(tp)

    result = mt5.order_send(request)
    if result is None:
        return {"error": f"Order send failed: {mt5.last_error()}"}

    if result.retcode != mt5.TRADE_RETCODE_DONE:
        return {"error": f"Order rejected: {result.comment}", "retcode": result.retcode}

    return {
        "ok": True,
        "order_id": str(result.order),
        "deal_id": str(result.deal),
        "price": result.price,
        "volume": result.volume,
    }


def handle_cancel_order(params):
    ticket = int(params["order_id"])
    request = {
        "action": mt5.TRADE_ACTION_REMOVE,
        "order": ticket,
    }
    result = mt5.order_send(request)
    if result is None:
        return {"error": f"Cancel failed: {mt5.last_error()}"}
    if result.retcode != mt5.TRADE_RETCODE_DONE:
        return {"error": f"Cancel rejected: {result.comment}", "retcode": result.retcode}
    return {"ok": True}


def handle_symbols(_params):
    symbols = mt5.symbols_get()
    if symbols is None:
        return {"symbols": []}
    result = []
    for s in symbols:
        if s.visible:
            result.append({
                "name": s.name,
                "description": s.description,
                "point": s.point,
                "digits": s.digits,
                "spread": s.spread,
                "trade_mode": s.trade_mode,
            })
    return {"symbols": result, "count": len(result)}


HANDLERS = {
    "init": handle_init,
    "shutdown": handle_shutdown,
    "get_price": handle_get_price,
    "get_candles": handle_get_candles,
    "get_balances": handle_get_balances,
    "get_positions": handle_get_positions,
    "get_open_orders": handle_get_open_orders,
    "place_order": handle_place_order,
    "cancel_order": handle_cancel_order,
    "symbols": handle_symbols,
}


def main():
    parser = argparse.ArgumentParser(description="MT5 Bridge for Clawtrade")
    parser.add_argument("--terminal", help="Path to MT5 terminal executable")
    args = parser.parse_args()

    # Auto-init if terminal path provided
    if args.terminal:
        result = handle_init({"terminal": args.terminal})
        respond({"id": "auto_init", "result": result})

    # Main loop: read JSON commands from stdin
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue

        try:
            cmd = json.loads(line)
        except json.JSONDecodeError as e:
            respond({"id": None, "error": f"Invalid JSON: {e}"})
            continue

        cmd_id = cmd.get("id")
        cmd_type = cmd.get("cmd")
        params = cmd.get("params", {})

        handler = HANDLERS.get(cmd_type)
        if handler is None:
            respond({"id": cmd_id, "error": f"Unknown command: {cmd_type}"})
            continue

        try:
            result = handler(params)
            respond({"id": cmd_id, "result": result})
        except Exception as e:
            respond({"id": cmd_id, "error": str(e)})

    # Cleanup
    mt5.shutdown()


if __name__ == "__main__":
    main()
