import { ssr } from "./reactive_gen";

const out = document.getElementById("star-result");
document.getElementById("star")?.addEventListener("click", () => {
  ssr
    .call("star", { starred: true })
    .then((r) => {
      if (out) out.textContent = `starred ${r.title}`;
    })
    .catch((e: { status: number }) => {
      if (out) out.textContent = `failed ${e.status}`;
    });
});
