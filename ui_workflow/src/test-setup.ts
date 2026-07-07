// Polyfills and global mocks for jsdom test environment.
// jsdom does not implement EventSource, so we provide a controllable mock
// that tests can drive synchronously.

import { afterEach, vi } from "vitest";

// Each test gets a fresh map of created EventSource instances so tests
// can grab the instance and drive .onopen / .onmessage / .onerror manually.
export const __mockEventSourceInstances: MockEventSource[] = [];

export class MockEventSource {
	url: string;
	readyState = 0; // CONNECTING
	onopen: ((ev: Event) => void) | null = null;
	onmessage: ((ev: MessageEvent) => void) | null = null;
	onerror: ((ev: Event) => void) | null = null;
	private listeners = new Map<string, ((ev: MessageEvent) => void)[]>();
	close = vi.fn(() => {
		this.readyState = 2; // CLOSED
	});

	constructor(url: string) {
		this.url = url;
		__mockEventSourceInstances.push(this);
	}

	addEventListener(type: string, handler: (ev: MessageEvent) => void) {
		const arr = this.listeners.get(type) ?? [];
		arr.push(handler);
		this.listeners.set(type, arr);
	}

	removeEventListener(type: string, handler: (ev: MessageEvent) => void) {
		const arr = this.listeners.get(type) ?? [];
		this.listeners.set(
			type,
			arr.filter((h) => h !== handler),
		);
	}

	// Test helpers
	__open() {
		this.readyState = 1;
		this.onopen?.(new Event("open"));
	}
	__emit(type: string, data: unknown) {
		const ev = new MessageEvent(type, { data: JSON.stringify(data) });
		this.listeners.get(type)?.forEach((h) => {
			h(ev);
		});
		if (type === "message") this.onmessage?.(ev);
	}
	__error() {
		this.onerror?.(new Event("error"));
	}
}

// Stub global navigation for 401 redirect tests
Object.defineProperty(window, "location", {
	writable: true,
	value: { href: "", assign: vi.fn(), replace: vi.fn() },
});

// Install EventSource polyfill before any module imports it
(globalThis as unknown as { EventSource: typeof MockEventSource }).EventSource =
	MockEventSource;

// Reset between tests
afterEach(() => {
	__mockEventSourceInstances.length = 0;
	(window.location as unknown as { href: string }).href = "";
});