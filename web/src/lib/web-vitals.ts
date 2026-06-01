/**
 * Core Web Vitals reporting module (SF-ENG0033).
 *
 * Only active in production (import.meta.env.PROD).
 * Uses the web-vitals library to collect LCP, INP, CLS metrics
 * and reports them to POST /api/metrics/web-vitals.
 */

import { onLCP, onINP, onCLS, type Metric } from "web-vitals";

const API_ENDPOINT = "/api/metrics/web-vitals";

// Whether reporting is enabled (prod only)
const isEnabled = import.meta.env.PROD;

interface WebVitalPayload {
  name: Metric["name"];
  value: number;
  rating: Metric["rating"];
  path: string;
  navigationType: string;
}

/**
 * Report a single metric to the backend.
 * Uses sendBeacon for reliability; falls back to fetch.
 */
function reportMetric(payload: WebVitalPayload): void {
  const body = JSON.stringify(payload);

  // Prefer sendBeacon (fire-and-forget, survives page unload)
  if (navigator.sendBeacon) {
    navigator.sendBeacon(API_ENDPOINT, body);
    return;
  }

  // Fallback: fetch with keepalive
  fetch(API_ENDPOINT, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body,
    keepalive: true,
  }).catch(() => {
    // Silently ignore — metrics should never block user experience
  });
}

/**
 * Get the navigation type from the Performance API.
 */
function getNavigationType(): string {
  const entries = performance.getEntriesByType("navigation");
  if (entries.length > 0) {
    const entry = entries[0] as PerformanceNavigationTiming;
      switch (entry.type as string) {
      case "navigate":
        return "navigate";
      case "reload":
        return "reload";
      case "back_forward":
        return "back-forward";
      case "prerender":
        return "prerender";
    }
  }
  return "unknown";
}

/**
 * Initialize Core Web Vitals collection and reporting.
 * Call this once in main.tsx after the app renders.
 */
export function initWebVitals(): void {
  if (!isEnabled) {
    return;
  }

  const navigationType = getNavigationType();
  const path = window.location.pathname;

  const handleMetric = (metric: Metric): void => {
    reportMetric({
      name: metric.name,
      value: metric.value,
      rating: metric.rating,
      path,
      navigationType,
    });
  };

  onLCP(handleMetric);
  onINP(handleMetric);
  onCLS(handleMetric);
}
