/**
 * Low-level HTTP client for GoClaw API.
 * Handles auth headers, retry with exponential backoff, circuit breaker, and error mapping.
 */

import { GoClawError, type ApiErrorData } from "../lib/errors.js";
import { logger } from "../lib/logger.js";
import type { ApiResponse } from "./types.js";

export interface HttpClientOptions {
  baseUrl: string;
  token?: string;
  userId?: string;
}

/** Circuit breaker states */
type CircuitState = "closed" | "open" | "half-open";

const MAX_RETRIES = 3;
const BASE_DELAY_MS = 200;
const MAX_DELAY_MS = 5000;
const REQUEST_TIMEOUT_MS = 30_000;
const CIRCUIT_FAILURE_THRESHOLD = 5;
const CIRCUIT_COOLDOWN_MS = 30_000;

export class HttpClient {
  private baseUrl: string;
  private token?: string;
  private userId?: string;

  // Circuit breaker state
  private circuitState: CircuitState = "closed";
  private failureCount = 0;
  private lastFailureTime = 0;

  constructor(options: HttpClientOptions) {
    this.baseUrl = options.baseUrl;
    this.token = options.token;
    this.userId = options.userId;
  }

  async get<T>(path: string): Promise<T> {
    return this.request<T>("GET", path);
  }

  async post<T>(path: string, body?: unknown): Promise<T> {
    return this.request<T>("POST", path, body);
  }

  async put<T>(path: string, body?: unknown): Promise<T> {
    return this.request<T>("PUT", path, body);
  }

  async patch<T>(path: string, body?: unknown): Promise<T> {
    return this.request<T>("PATCH", path, body);
  }

  async del<T = void>(path: string): Promise<T> {
    return this.request<T>("DELETE", path);
  }

  private async request<T>(method: string, path: string, body?: unknown): Promise<T> {
    this.checkCircuit();

    const url = `${this.baseUrl}${path}`;
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
      Accept: "application/json",
    };

    if (this.token) {
      headers["Authorization"] = `Bearer ${this.token}`;
    }
    if (this.userId) {
      headers["X-GoClaw-User-Id"] = this.userId;
    }

    let lastError: Error | undefined;

    for (let attempt = 0; attempt <= MAX_RETRIES; attempt++) {
      try {
        const controller = new AbortController();
        const timeout = setTimeout(() => controller.abort(), REQUEST_TIMEOUT_MS);

        const response = await fetch(url, {
          method,
          headers,
          body: body ? JSON.stringify(body) : undefined,
          signal: controller.signal,
        });

        clearTimeout(timeout);

        logger.debug(`${method} ${path}`, { status: response.status });

        // Retry on 429 or 5xx
        if ((response.status === 429 || response.status >= 500) && attempt < MAX_RETRIES) {
          lastError = new Error(`HTTP ${response.status}`);
          await this.backoff(attempt);
          continue;
        }

        // Parse response
        const data = (await response.json()) as ApiResponse<T>;

        if (!data.ok && data.error) {
          this.onFailure();
          throw new GoClawError(data.error as ApiErrorData);
        }

        this.onSuccess();
        return data.payload;
      } catch (err) {
        if (err instanceof GoClawError) throw err;

        lastError = err instanceof Error ? err : new Error(String(err));

        // Don't retry on abort or client errors
        if (lastError.name === "AbortError") {
          this.onFailure();
          throw new GoClawError({
            code: "ERR_TIMEOUT",
            message: `Request timed out after ${REQUEST_TIMEOUT_MS}ms: ${method} ${path}`,
            status_code: 408,
          });
        }

        if (attempt < MAX_RETRIES) {
          await this.backoff(attempt);
          continue;
        }
      }
    }

    this.onFailure();
    throw new GoClawError({
      code: "ERR_CONNECTION",
      message: `Failed to connect to GoClaw after ${MAX_RETRIES + 1} attempts: ${lastError?.message ?? "unknown error"}`,
      status_code: 502,
    });
  }

  /** Exponential backoff with jitter */
  private async backoff(attempt: number): Promise<void> {
    const delay = Math.min(BASE_DELAY_MS * Math.pow(2, attempt), MAX_DELAY_MS);
    const jitter = delay * (0.7 + Math.random() * 0.3);
    await new Promise((r) => setTimeout(r, jitter));
  }

  /** Check circuit breaker before making a request */
  private checkCircuit(): void {
    if (this.circuitState === "open") {
      if (Date.now() - this.lastFailureTime > CIRCUIT_COOLDOWN_MS) {
        this.circuitState = "half-open";
        logger.info("Circuit breaker: half-open, testing recovery");
      } else {
        throw new GoClawError({
          code: "ERR_CIRCUIT_OPEN",
          message: "GoClaw API circuit breaker is open — too many consecutive failures. Will retry automatically.",
          status_code: 503,
        });
      }
    }
  }

  private onSuccess(): void {
    if (this.circuitState !== "closed") {
      logger.info("Circuit breaker: closed (recovered)");
    }
    this.circuitState = "closed";
    this.failureCount = 0;
  }

  private onFailure(): void {
    this.failureCount++;
    this.lastFailureTime = Date.now();
    if (this.failureCount >= CIRCUIT_FAILURE_THRESHOLD) {
      this.circuitState = "open";
      logger.warn("Circuit breaker: open", {
        failures: this.failureCount,
        cooldownMs: CIRCUIT_COOLDOWN_MS,
      });
    }
  }
}
