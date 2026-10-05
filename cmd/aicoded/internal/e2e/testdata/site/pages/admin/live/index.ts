import { ssr } from "./reactive_gen";

const error = document.getElementById("error");
ssr.onError((_name, message) => {
  if (error) error.textContent = message;
});
