import type { Connection } from "./connection";

const delay = 150;

// wireBindings sends the value of every element marked data-ssr-write when the viewer changes
// it. Listening on the document also covers elements that live blocks replace.
export function wireBindings(conn: Connection): void {
  const timers = new WeakMap<Element, number>();
  document.addEventListener("input", (e) => {
    const el = e.target;
    if (!(el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement || el instanceof HTMLSelectElement)) return;
    if (!el.hasAttribute("data-ssr-write")) return;
    const key = el.getAttribute("data-ssr-bind") ?? "";
    const dot = key.indexOf(".");
    if (dot < 0) return;
    window.clearTimeout(timers.get(el));
    timers.set(
      el,
      window.setTimeout(() => {
        const value = el instanceof HTMLInputElement && el.type === "checkbox" ? String(el.checked) : el.value;
        conn.send({ t: "write", routeKey: key.slice(0, dot), var: key.slice(dot + 1), value });
      }, delay),
    );
  });
}
