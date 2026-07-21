import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterEach } from "vitest";

afterEach(cleanup);

Object.defineProperty(Element.prototype, "scrollIntoView", { value: () => {}, writable: true });
globalThis.requestAnimationFrame = (callback: FrameRequestCallback) => { callback(0); return 0; };

// Node 26 exposes an unavailable experimental localStorage unless launched
// with a backing file. Keep browser tests isolated and deterministic instead.
const storageValues = new Map<string, string>();
const memoryStorage: Storage = {
  get length() { return storageValues.size; },
  clear: () => storageValues.clear(),
  getItem: (key) => storageValues.get(key) ?? null,
  key: (index) => [...storageValues.keys()][index] ?? null,
  removeItem: (key) => { storageValues.delete(key); },
  setItem: (key, value) => { storageValues.set(key, String(value)); },
};
Object.defineProperty(globalThis, "localStorage", { value: memoryStorage, configurable: true });
afterEach(() => memoryStorage.clear());
