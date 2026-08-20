/**
 * Token bucket rate limiter — per-session rate limiting for HTTP transport.
 * Returns 429 when bucket is exhausted.
 */

interface Bucket {
  tokens: number;
  lastRefill: number;
}

export class RateLimiter {
  private buckets = new Map<string, Bucket>();
  private maxTokens: number;
  private refillRatePerMs: number;
  private cleanupInterval: ReturnType<typeof setInterval>;

  /**
   * @param rpm Requests per minute allowed per session
   */
  constructor(rpm: number) {
    this.maxTokens = Math.max(rpm, 10); // burst = full minute capacity
    this.refillRatePerMs = rpm / 60_000;

    // Clean up stale buckets every 5 minutes
    this.cleanupInterval = setInterval(() => this.cleanup(), 5 * 60_000);
    // Don't block process exit
    if (this.cleanupInterval.unref) this.cleanupInterval.unref();
  }

  /**
   * Check if request is allowed for the given session.
   * @returns true if allowed, false if rate limited
   */
  allow(sessionId: string): boolean {
    const now = Date.now();
    let bucket = this.buckets.get(sessionId);

    if (!bucket) {
      bucket = { tokens: this.maxTokens - 1, lastRefill: now };
      this.buckets.set(sessionId, bucket);
      return true;
    }

    // Refill tokens based on elapsed time
    const elapsed = now - bucket.lastRefill;
    bucket.tokens = Math.min(this.maxTokens, bucket.tokens + elapsed * this.refillRatePerMs);
    bucket.lastRefill = now;

    if (bucket.tokens >= 1) {
      bucket.tokens -= 1;
      return true;
    }

    return false;
  }

  /** Seconds until next token is available */
  retryAfter(sessionId: string): number {
    const bucket = this.buckets.get(sessionId);
    if (!bucket) return 0;
    const tokensNeeded = 1 - bucket.tokens;
    return Math.ceil(tokensNeeded / this.refillRatePerMs / 1000);
  }

  /** Remove session bucket */
  remove(sessionId: string): void {
    this.buckets.delete(sessionId);
  }

  /** Clean up buckets idle for >10 minutes */
  private cleanup(): void {
    const cutoff = Date.now() - 10 * 60_000;
    for (const [id, bucket] of this.buckets) {
      if (bucket.lastRefill < cutoff) {
        this.buckets.delete(id);
      }
    }
  }

  destroy(): void {
    clearInterval(this.cleanupInterval);
    this.buckets.clear();
  }
}
