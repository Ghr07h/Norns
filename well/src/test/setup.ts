import "@testing-library/jest-dom/vitest";
import "fake-indexeddb/auto";
import "../i18n";

Object.defineProperty(navigator, "clipboard", { value: { writeText: async () => undefined }, configurable: true });
