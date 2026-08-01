import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import ErrorBoundary from "@/shared/components/ErrorBoundary";
import "./index.css";
import App from "./App.tsx";
import { initWebVitals } from "@/shared/lib/web-vitals";

// Initialize Core Web Vitals reporting (production only)
initWebVitals();

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ErrorBoundary title="应用出现了问题">
      <App />
    </ErrorBoundary>
  </StrictMode>,
);
