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
