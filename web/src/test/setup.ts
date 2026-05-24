import "@testing-library/jest-dom";
import { cleanup } from "@testing-library/react";

// Auto-cleanup between tests
afterEach(() => {
  cleanup();
});

// Polyfills required by cmdk and other browser APIs in jsdom

global.ResizeObserver = class ResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
};

global.IntersectionObserver = class IntersectionObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
  takeRecords() {
    return [];
  }
};

// cmdk calls scrollIntoView in jsdom which doesn't implement it
Element.prototype.scrollIntoView = function () {};

// matchMedia polyfill for jsdom
Object.defineProperty(window, "matchMedia", {
  writable: true,
  value: (query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  }),
});
