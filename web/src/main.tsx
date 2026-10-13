import "./styles/globals.css";

import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { App } from "./app/App";
import { installScrollCueFallback } from "./shared/lib/scroll-cue";

const root = document.getElementById("root");
if (!root) throw new Error("Missing #root element");

installScrollCueFallback();

createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
