import { describe, it, expect, afterEach } from "vitest";
import { RateLimiter } from "../../src/lib/rate-limiter.js";

describe("RateLimiter", () => {
  let limiter: RateLimiter;

  afterEach(() => {
    limiter?.destroy();
  });

  it("allows requests within limit", () => {
    limiter = new RateLimiter(60);
    for (let i = 0; i < 60; i++) {
      expect(limiter.allow("session-1")).toBe(true);
    }
  });

  it("blocks requests exceeding limit", () => {
    limiter = new RateLimiter(10);
    // Exhaust all tokens
    for (let i = 0; i < 10; i++) {
      limiter.allow("session-1");
    }
    expect(limiter.allow("session-1")).toBe(false);
  });

  it("tracks sessions independently", () => {
    limiter = new RateLimiter(10);
    for (let i = 0; i < 10; i++) {
      limiter.allow("session-a");
    }
    expect(limiter.allow("session-a")).toBe(false);
    expect(limiter.allow("session-b")).toBe(true);
  });

  it("provides retry-after seconds", () => {
    limiter = new RateLimiter(10);
    for (let i = 0; i < 10; i++) {
      limiter.allow("session-1");
    }
    const retryAfter = limiter.retryAfter("session-1");
    expect(retryAfter).toBeGreaterThan(0);
  });

  it("removes session bucket", () => {
    limiter = new RateLimiter(10);
    for (let i = 0; i < 10; i++) {
      limiter.allow("session-1");
    }
    expect(limiter.allow("session-1")).toBe(false);
    limiter.remove("session-1");
    // After removal, new bucket starts fresh
    expect(limiter.allow("session-1")).toBe(true);
  });
});
