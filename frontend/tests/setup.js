import { afterEach, vi } from "vitest";
import { cleanup } from "@testing-library/react";

Object.defineProperty(HTMLElement.prototype, "scrollIntoView", {
  configurable: true,
  value: vi.fn(),
});
Object.defineProperty(window, "matchMedia", {
  configurable: true,
  value: vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })),
});

afterEach(cleanup);
