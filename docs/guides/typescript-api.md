# TypeScript API: the ssr object

Live values and page calls work without any script. Write one in `index.ts` when a page needs
more: showing why a value was refused, reacting to a lost connection, calling a page call, or
writing a value that no input holds. The page's generated `reactive_gen.ts` gives the script a
typed `ssr` object.

## The generated file

A page with live values or page calls gets `reactive_gen.ts` next to its template:

```ts
export type ReadVars = {
  displayName: string;
  visitorsOnline: number;
  visitorsTone: string;
};

export type WriteVars = {
  displayName: string;
};

export type Calls = Record<string, never>;

export interface CallError {
  status: number;
  message: string;
}

export const ssr = …;
```

- `ReadVars` lists every live value of the page with its type in TypeScript, and `WriteVars`
  the client-writable ones.
- `Calls` lists the page's calls with their `in` and `out` types, which are declared above it.
- Never edit it: `aicoded generate` writes it again from the template.

The page's script imports it:

```ts
import { ssr } from "./reactive_gen";
```

The build strips the types without checking them, so a mistake shows in your editor, not in
`aicoded check`.

## The ssr object

| Call | What it does |
|---|---|
| `ssr.get(name)` | the text last shown for the live value `name`, or `undefined` |
| `ssr.on(name, (value) => …)` | runs the callback each time the live value `name` is shown again |
| `ssr.set(name, value)` | writes a client-writable value; only names in `WriteVars` |
| `ssr.onError((name, message) => …)` | runs when the server refuses a written value |
| `ssr.call(name, args)` | runs a page call; see [page calls](page-calls.md) |
| `ssr.onConnect(() => …)` | runs each time the live connection opens |
| `ssr.onDisconnect((code) => …)` | runs each time it closes, with the WebSocket close code |

Every `on…` returns a function that removes the callback.

- `get` and `on` name a live value that the page shows alone, as `{{ displayName }}` in text, or
  that an input binds with `ssr:bind`. A value shown as part of something bigger, such as
  `{{ a + b }}`, an attribute or an `ssr:if` chain, is updated in place but has no name here.
- The value they give is the text the page shows, not the Go value, and the page already shows
  it when the callback runs. Use them for side effects, not to render.
- `set` sends the value and returns at once. The value comes back as an update once
  `Validate<Name>` accepted it, or as an `onError` with its message. A `set` while the connection
  is closed is dropped.

## An example

The home page of the people example shows why a display name was refused, clears the message
when a name is accepted, and reads data the template passed with `<ssr:json>`:

<!-- code: examples/people/pages/home/index.ts -->
```ts
import { ssr } from "./reactive_gen";

const error = document.getElementById("display-name-error");
ssr.onError((name, message) => {
  if (name === "displayName" && error) error.textContent = message;
});
ssr.on("displayName", () => {
  if (error) error.textContent = "";
});

const langs: string[] = JSON.parse(document.getElementById("langs-data")?.textContent ?? "[]");
const note = document.getElementById("script-note");
if (note) note.textContent = `A script read ${langs.length} languages from the page.`;
```

## The connection

- The page opens its live connection when the document is parsed, after its scripts ran, so a
  script's callbacks see the first values and the first connect.
- When the connection closes with any code but 1000 or 1001, the page connects again, with a
  growing delay of up to 30 seconds, and at once after code 4000, which the server sends when a
  connection reached its 10 minutes. On every connect the page gets every current value. After
  1000 or 1001 it stays closed, and every waiting call fails.
- `onDisconnect` gets the close code; show a notice from it and hide it again in `onConnect`.

## See also

- [Live values](live-values.md): what the server sends and checks.
- [Page calls](page-calls.md): `ssr.call` and its errors.
- [Assets](assets.md): how `index.ts` is built and loaded.
