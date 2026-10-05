import { wireBindings } from "./bind";
import { Connection } from "./connection";
import "./reactive.css";

interface Route {
  get(name: string): string | undefined;
  set(name: string, value: unknown): void;
  on(name: string, callback: (value: string) => void): () => void;
  onError(callback: (name: string, message: string) => void): () => void;
  onConnect(callback: () => void): () => void;
  onDisconnect(callback: (code: number) => void): () => void;
  call(name: string, args: unknown): Promise<unknown>;
}

// The connection carries the page's query, so the page's hooks see the same request.
const scheme = location.protocol === "https:" ? "wss:" : "ws:";
const conn = new Connection(`${scheme}//${location.host}${location.pathname.replace(/\/$/, "")}/__ws${location.search}`);

function route(routeKey: string): Route {
  return {
    get: (name) => conn.values.get(`${routeKey}.${name}`),
    set: (name, value) => conn.send({ t: "write", routeKey, var: name, value }),
    on: (name, callback) =>
      conn.changed.add((key, value) => {
        if (key === `${routeKey}.${name}`) callback(value);
      }),
    onError: (callback) =>
      conn.errors.add((key, name, message) => {
        if (key === routeKey) callback(name, message);
      }),
    onConnect: (callback) => conn.connected.add(callback),
    onDisconnect: (callback) => conn.disconnected.add(callback),
    call: (name, args) => conn.call(routeKey, name, args),
  };
}

(globalThis as unknown as { aicodedReactive: { route: typeof route } }).aicodedReactive = { route };
wireBindings(conn);
// Page scripts run after this one and before DOMContentLoaded; opening then lets them see the
// first values and the first connect.
if (document.readyState === "complete") conn.open();
else document.addEventListener("DOMContentLoaded", () => conn.open(), { once: true });
