import { label } from "./label";

document.querySelectorAll<HTMLElement>("[data-label]").forEach((el) => {
  el.textContent = label(el.dataset.label ?? "");
});
