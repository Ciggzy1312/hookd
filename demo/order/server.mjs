#!/usr/bin/env node
// Order service — POST /orders emits order.paid to the payment service.
// POST /webhooks/payment (and POST /i/{id} for hookd --forward) marks the order paid.

import { createServer } from "node:http";
import { randomUUID } from "node:crypto";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const env = loadEnv(join(dirname(fileURLToPath(import.meta.url)), ".env"));
const ADDR = env.ADDR || "127.0.0.1:3001";
const PAYMENT_URL = env.PAYMENT_URL || "http://127.0.0.1:3002/events/order-paid";

const orders = new Map();

const server = createServer(async (req, res) => {
  try {
    const path = new URL(req.url, "http://localhost").pathname;
    if (req.method === "GET" && path === "/") return json(res, 200, info());
    if (req.method === "GET" && path === "/health") return json(res, 200, { ok: true });
    if (req.method === "GET" && path === "/orders") return json(res, 200, [...orders.values()]);
    if (req.method === "GET" && path.startsWith("/orders/")) {
      const order = orders.get(path.slice("/orders/".length));
      if (!order) return json(res, 404, { error: "order not found" });
      return json(res, 200, order);
    }
    if (req.method === "POST" && path === "/orders") return handleCreate(req, res);
    if (req.method === "POST" && (path === "/webhooks/payment" || path.startsWith("/i/"))) {
      return handleWebhook(req, res);
    }
    json(res, 404, { error: "not found" });
  } catch (err) {
    console.error("handler", err);
    json(res, 500, { error: "internal" });
  }
});

async function handleCreate(req, res) {
  const body = await readJSON(req);
  const amount = Number(body.amount);
  if (!body.item || !Number.isFinite(amount) || amount <= 0) {
    return json(res, 400, { error: "item and positive amount required" });
  }

  const order = {
    id: "ord_" + randomUUID().replaceAll("-", "").slice(0, 12),
    item: String(body.item),
    amount: Math.round(amount),
    currency: body.currency || "usd",
    status: "pending",
    payment_id: null,
    created_at: new Date().toISOString(),
  };
  orders.set(order.id, order);

  const event = {
    type: "order.paid",
    order_id: order.id,
    item: order.item,
    amount: order.amount,
    currency: order.currency,
    paid_at: order.created_at,
  };

  try {
    const resp = await fetch(PAYMENT_URL, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Event-Type": "order.paid",
        "X-Order-Id": order.id,
      },
      body: JSON.stringify(event),
      signal: AbortSignal.timeout(10_000),
    });
    if (!resp.ok) {
      const text = await resp.text();
      console.error("payment rejected", resp.status, text);
      return json(res, 502, { error: "payment service rejected order.paid", status: resp.status });
    }
  } catch (err) {
    console.error("payment unreachable", err.message);
    return json(res, 502, { error: "payment service unreachable" });
  }

  console.log("order created", order.id, order.item, order.amount);
  json(res, 201, order);
}

async function handleWebhook(req, res) {
  const body = await readJSON(req);
  const data = body.data || body;
  const orderID = data.order_id;
  const order = orders.get(orderID);
  if (!order) {
    return json(res, 404, { error: "order not found", order_id: orderID });
  }

  order.status = "paid";
  order.payment_id = data.payment_id || data.id || order.payment_id;
  order.paid_at = new Date().toISOString();
  console.log("webhook payment.succeeded", order.id, order.payment_id);
  json(res, 200, { ok: true, order });
}

function info() {
  return {
    service: "order",
    endpoints: {
      "POST /orders": "create an order and emit order.paid to payment",
      "GET /orders": "list orders",
      "POST /webhooks/payment": "payment.succeeded (replay target)",
      "POST /i/{id}": "same webhook, for hookd --forward (path is preserved)",
    },
  };
}

function json(res, status, body) {
  const raw = JSON.stringify(body);
  res.writeHead(status, {
    "Content-Type": "application/json; charset=utf-8",
    "Content-Length": Buffer.byteLength(raw),
  });
  res.end(raw);
}

async function readJSON(req) {
  const chunks = [];
  for await (const chunk of req) chunks.push(chunk);
  const raw = Buffer.concat(chunks).toString("utf8").trim();
  if (!raw) return {};
  return JSON.parse(raw);
}

function loadEnv(path) {
  let text = "";
  try {
    text = readFileSync(path, "utf8");
  } catch {
    return {};
  }
  const out = {};
  for (const line of text.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("#")) continue;
    const i = trimmed.indexOf("=");
    if (i < 0) continue;
    const k = trimmed.slice(0, i).trim();
    const v = trimmed.slice(i + 1).trim().replace(/^['"]|['"]$/g, "");
    if (k) out[k] = v;
  }
  return out;
}

server.listen(...splitAddr(ADDR), () => {
  console.log(`order    http://${ADDR}/`);
  console.log(`  pay -> ${PAYMENT_URL}`);
  console.log(`  hook <- POST /webhooks/payment  (and POST /i/{id})`);
});

function splitAddr(addr) {
  const i = addr.lastIndexOf(":");
  return [Number(addr.slice(i + 1)), addr.slice(0, i)];
}
