import type { Binding, CallError, CallFrame, ServerFrame, WriteFrame } from "./protocol";
import { StatusReauth } from "./protocol";

interface Pending {
  resolve: (value: unknown) => void;
  reject: (error: CallError) => void;
}

type Listener<T extends unknown[]> = (...args: T) => void;

export class Listeners<T extends unknown[]> {
  private readonly items = new Set<Listener<T>>();

  add(fn: Listener<T>): () => void {
    this.items.add(fn);
    return () => {
      this.items.delete(fn);
    };
  }

  // emit calls every listener; one that throws is reported and does not stop the others.
  emit(...args: T): void {
    for (const fn of this.items) {
      try {
        fn(...args);
      } catch (err) {
        reportError(err);
      }
    }
  }
}

const maxBackoff = 30000;

function lost(): CallError {
  return { status: 0, message: "The connection was lost." };
}

// Connection is the page's one live connection. It shows text values as text and block values,
// which the server rendered with the page's escapers, as markup. Calls made before it opens
// wait for it; calls in flight when it drops fail with status 0 and are not retried.
//
// A close with 1000 or 1001 is final: it fails every waiting call, and every later call fails
// at once. After any other close it reconnects with a growing delay of up to 30 s, and at once
// after StatusReauth. The delay starts again only after a connection stayed open for 30 s, so
// a page the server refuses, or opens and closes at once, is not retried more often.
export class Connection {
  readonly values = new Map<string, string>();
  readonly changed = new Listeners<[key: string, value: string]>();
  readonly errors = new Listeners<[routeKey: string, name: string, message: string]>();
  readonly connected = new Listeners<[]>();
  readonly disconnected = new Listeners<[code: number]>();
  private ws: WebSocket | null = null;
  private retries = 0;
  private closed = false;
  private nextID = 1;
  private readonly pending = new Map<number, Pending>();
  private readonly sent = new Set<number>();
  private readonly queued: CallFrame[] = [];

  constructor(private readonly url: string) {}

  open(): void {
    const ws = new WebSocket(this.url);
    let opened = 0;
    this.ws = ws;
    ws.onopen = () => {
      opened = Date.now();
      for (const frame of this.queued.splice(0)) this.sendCall(frame);
      this.connected.emit();
    };
    ws.onmessage = (e) => this.receive(JSON.parse(String(e.data)) as ServerFrame);
    ws.onclose = (e) => {
      this.ws = null;
      if (opened > 0 && Date.now() - opened >= maxBackoff) this.retries = 0;
      this.closed = e.code === 1000 || e.code === 1001;
      if (this.closed) this.queued.length = 0;
      for (const id of this.closed ? [...this.pending.keys()] : [...this.sent]) this.settle(id, (p) => p.reject(lost()));
      this.disconnected.emit(e.code);
      if (this.closed) return;
      const backoff = Math.min(maxBackoff, 500 * 2 ** this.retries++) * (0.5 + Math.random() / 2);
      window.setTimeout(() => this.open(), e.code === StatusReauth ? 0 : backoff);
    };
  }

  send(frame: WriteFrame): void {
    if (this.ws?.readyState === WebSocket.OPEN) this.ws.send(JSON.stringify(frame));
  }

  call(routeKey: string, name: string, args: unknown): Promise<unknown> {
    if (this.closed) return Promise.reject(lost());
    const frame: CallFrame = { t: "call", id: this.nextID++, routeKey, name, args: args ?? null };
    return new Promise((resolve, reject) => {
      this.pending.set(frame.id, { resolve, reject });
      if (this.ws?.readyState === WebSocket.OPEN) this.sendCall(frame);
      else this.queued.push(frame);
    });
  }

  private sendCall(frame: CallFrame): void {
    this.ws?.send(JSON.stringify(frame));
    this.sent.add(frame.id);
  }

  private settle(id: number, done: (p: Pending) => void): void {
    const p = this.pending.get(id);
    if (!p) return;
    this.pending.delete(id);
    this.sent.delete(id);
    done(p);
  }

  private receive(frame: ServerFrame): void {
    switch (frame.t) {
      case "init":
        for (const [key, b] of Object.entries(frame.bindings)) this.apply(key, b);
        break;
      case "patch":
        this.apply(frame.key, frame);
        break;
      case "err":
        this.errors.emit(frame.routeKey, frame.var, frame.msg);
        break;
      case "result":
        this.settle(frame.id, (p) => p.resolve(frame.value));
        break;
      case "fail":
        this.settle(frame.id, (p) => p.reject({ status: frame.status, message: frame.msg }));
        break;
    }
  }

  // apply shows a value in every element bound to key. An element that throws, such as a file
  // input given a value, is reported and skipped, so the others still get theirs.
  private apply(key: string, b: Binding): void {
    this.values.set(key, b.value);
    for (const el of document.querySelectorAll<HTMLElement>(`[data-ssr-bind="${CSS.escape(key)}"]`)) {
      try {
        show(el, b);
      } catch (err) {
        reportError(err);
      }
    }
    this.changed.emit(key, b.value);
  }
}

// show puts a value into one bound element. Only a block value, which the server rendered with
// the page's escapers, becomes markup.
function show(el: HTMLElement, b: Binding): void {
  if (b.kind === "html") {
    el.innerHTML = b.value;
  } else if (el instanceof HTMLInputElement && (el.type === "checkbox" || el.type === "radio")) {
    el.checked = el.type === "checkbox" ? b.value === "true" : el.value === b.value;
  } else if (el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement || el instanceof HTMLSelectElement) {
    if (document.activeElement !== el) el.value = b.value;
  } else {
    el.textContent = b.value;
  }
}
