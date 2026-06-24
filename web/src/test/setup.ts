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

// localStorage / sessionStorage polyfill — jsdom 在 Vitest 4 下提供的 storage
// 对象缺少 clear/getItem 等方法（getter 返回的对象不完整），用内存 mock 替换。
const createStorageMock = () => {
  const store: Record<string, string> = {};
  return {
    getItem: (key: string) => store[key] ?? null,
    setItem: (key: string, value: string) => {
      store[key] = String(value);
    },
    removeItem: (key: string) => {
      delete store[key];
    },
    clear: () => {
      Object.keys(store).forEach((k) => delete store[k]);
    },
    key: (index: number) => Object.keys(store)[index] ?? null,
    get length() {
      return Object.keys(store).length;
    },
  };
};

Object.defineProperty(window, "localStorage", {
  writable: true,
  value: createStorageMock(),
});
Object.defineProperty(window, "sessionStorage", {
  writable: true,
  value: createStorageMock(),
});
