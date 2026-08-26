import { describe, it, expect } from "vitest";
import { SERVER_NAME, SERVER_VERSION } from "../../src/version.js";
import { readdirSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const pkg = JSON.parse(readFileSync(join(here, "../../package.json"), "utf-8")) as {
  name: string;
  version: string;
  bin: Record<string, string>;
};

describe("goso-mcp identity", () => {
  it("uses name goso-mcp and version 0.1.0", () => {
    expect(SERVER_NAME).toBe("goso-mcp");
    expect(SERVER_VERSION).toBe("0.1.0");
    expect(pkg.name).toBe("goso-mcp");
    expect(pkg.version).toBe(SERVER_VERSION);
    expect(pkg.bin["goso-mcp"]).toBe("./dist/index.js");
    expect(pkg.bin["goso-mcp-http"]).toBe("./dist/http.js");
  });

  it("keeps 66 goso_* tools", () => {
    const dir = join(here, "../../src/tools");
    let count = 0;
    for (const f of readdirSync(dir)) {
      if (!f.startsWith("register-")) continue;
      const text = readFileSync(join(dir, f), "utf-8");
      count += (text.match(/server\.tool\(/g) ?? []).length;
    }
    expect(count).toBe(66);
  });
});
