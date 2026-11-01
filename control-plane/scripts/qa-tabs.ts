/**
 * Browser check for v2 §13 rule 2.
 * Usage: npm run qa:tabs
 * See docs/qa/qa-tabs.md.
 */
import { mkdirSync } from "node:fs";
import { FOLDED_HASHES, HASH_ALIASES, SIDEBAR_TABS, ignoredConsole, ignoredFailure, unfinishedMarker } from "../src/api/qa-tabs.ts";

const ui = (process.env.GOSO_UI || "http://127.0.0.1:3000").replace(/\/$/, "");
const gw = (process.env.GOSO_GW || "http://127.0.0.1:8080").replace(/\/$/, "");
const shots = process.env.QA_SHOTS === "1";
const shotDir = process.env.QA_SHOTS_DIR || "/tmp/goso-qa-tabs";
const protoFile = process.env.PROTO_HTML || "/home/ubuntu/.cursor/projects/workspace/uploads/Goso_Prototype_2026-11-01_870e.html";
const chrome = process.env.CHROME_PATH || "/usr/local/bin/google-chrome";

type Fail = { where: string; detail: string };
const fails: Fail[] = [];
const empties: string[] = [];

function fail(where: string, detail: string) {
  fails.push({ where, detail });
  console.log("FAIL", where, detail);
}

async function chromePath(): Promise<string> {
  const { access } = await import("node:fs/promises");
  try {
    await access(chrome);
    return chrome;
  } catch {
    return "google-chrome";
  }
}

async function main() {
  const health = await fetch(`${gw}/healthz`).catch(() => null);
  if (!health || health.status !== 200) {
    console.error(`gateway ${gw}/healthz is not 200. Start it before npm run qa:tabs.`);
    process.exit(1);
  }
  const puppeteer = await import("puppeteer-core");
  const browser = await puppeteer.launch({
    executablePath: await chromePath(),
    headless: true,
    args: ["--no-sandbox", "--disable-dev-shm-usage"],
    defaultViewport: { width: 1280, height: 800 },
  });
  const page = await browser.newPage();
  page.setDefaultTimeout(20000);
  let httpFails: string[] = [];
  let consoleFails: string[] = [];
  page.on("pageerror", (err) => consoleFails.push(String(err)));
  page.on("console", (msg) => {
    if (msg.type() !== "error") return;
    const text = msg.text();
    if (!ignoredConsole(text)) consoleFails.push(text);
  });
  page.on("response", (res) => {
    if (res.status() < 400) return;
    const url = res.url();
    if (ignoredFailure(url)) return;
    if (!/\/api\/|\/healthz/.test(url)) return;
    httpFails.push(`${res.status()} ${url}`);
  });

  async function openHash(hash: string, label: string) {
    httpFails = [];
    consoleFails = [];
    await page.goto(`${ui}/${hash}`, { waitUntil: "domcontentloaded" });
    await page.waitForSelector(".z-main, .z-gs", { timeout: 8000 });
    await new Promise((r) => setTimeout(r, 600));
    const text = await page.$eval(".z-shell", (el) => (el.textContent || "").replace(/\s+/g, " ").slice(0, 4000));
    const marker = unfinishedMarker(text);
    if (marker) fail(label, `unfinished marker “${marker}”`);
    if (/Chưa có (dữ liệu|cuộc họp|hook|nút)/.test(text)) empties.push(label);
    for (const row of consoleFails) fail(label, `console: ${row.slice(0, 300)}`);
    for (const row of httpFails) fail(label, `http: ${row}`);
    if (shots) {
      mkdirSync(shotDir, { recursive: true });
      const name = label.replace(/[^a-z0-9-]+/gi, "_");
      await page.screenshot({ path: `${shotDir}/goso-${name}.png` });
    }
    return text;
  }

  if (process.env.QA_SKIP_TABS === "1") {
    await chatFlows(page);
    await browser.close();
    if (fails.length) {
      console.error(`qa:tabs ${fails.length} failure(s)`);
      process.exit(1);
    }
    console.log("qa:tabs flow ok");
    return;
  }

  for (const tab of SIDEBAR_TABS) {
    console.log("tab", tab.hash);
    await openHash(tab.hash, tab.hash);
    const hash = await page.evaluate(() => location.hash);
    if (hash !== tab.hash && !(tab.hash === "#/config" && hash.startsWith("#/config"))) {
      fail(tab.hash, `landed on ${hash}`);
    }
  }
  for (const alias of HASH_ALIASES) {
    console.log("alias", alias.from);
    await openHash(alias.from, alias.from);
    const hash = await page.evaluate(() => location.hash);
    const norm = (value: string) => value.replace(/^#\/?/, "").split("/")[0];
    if (norm(hash) !== norm(alias.to)) fail(alias.from, `expected ${alias.to}, got ${hash}`);
  }
  for (const hash of FOLDED_HASHES) {
    console.log("folded", hash);
    await openHash(hash, hash);
  }

  if (shots) {
    const proto = await browser.newPage();
    const file = "file://" + protoFile;
    for (const tab of SIDEBAR_TABS) {
      await proto.goto(`${file}#${tab.proto}`, { waitUntil: "domcontentloaded" });
      await new Promise((r) => setTimeout(r, 250));
      await proto.screenshot({ path: `${shotDir}/proto-${tab.proto}.png` });
    }
    await proto.close();
  }

  await chatFlows(page);
  await browser.close();

  if (empties.length) console.log("empty (honest, not chưa dựng):", [...new Set(empties)].join(", "));
  if (fails.length) {
    console.error(`qa:tabs ${fails.length} failure(s)`);
    process.exit(1);
  }
  console.log("qa:tabs ok");
}

async function chatFlows(page: import("puppeteer-core").Page) {
  console.log("flow chat");
  await page.goto(`${ui}/#/chat`, { waitUntil: "domcontentloaded" });
  await page.waitForSelector(".z-gs-new");
  await page.click(".z-gs-new");
  await page.waitForSelector(".z-gs-form select");
  const agent = await page.$$eval(".z-gs-form select option", (nodes) => {
    const hit = nodes.find((n) => (n as HTMLOptionElement).value);
    return hit ? (hit as HTMLOptionElement).value : "";
  });
  if (!agent) {
    fail("chat", "no agent to start a conversation");
    return;
  }
  await page.select(".z-gs-form select", agent);
  const token = `qa${Date.now().toString().slice(-6)}`;
  const renamed = `Xong ${token}`;
  const label = await page.$(".z-gs-form input");
  if (label) {
    await label.click({ clickCount: 3 });
    await page.keyboard.type(`QA ${token}`);
  }
  await page.evaluate(() => {
    const button = [...document.querySelectorAll(".z-gs-form button")].find((el) => /Tạo/.test(el.textContent || ""));
    (button as HTMLButtonElement | undefined)?.click();
  });
  await page.waitForFunction(() => {
    const el = document.querySelector(".z-gs-comp textarea") as HTMLTextAreaElement | null;
    return Boolean(el && !el.disabled);
  });
  await page.click("[data-chat-agent]");
  await page.waitForSelector(".z-gs-pop");
  await page.click(".z-topbar");
  await page.click("[data-chat-model]");
  await page.waitForSelector(".z-gs-pop");
  await page.click(".z-topbar");

  const box = await page.$(".z-gs-comp textarea");
  if (!box) {
    fail("chat", "composer missing");
    return;
  }
  await box.click();
  await page.keyboard.type("@");
  await page.waitForSelector("[data-chat-mention]");
  await page.keyboard.press("Escape");
  await page.evaluate(() => {
    const el = document.querySelector(".z-gs-comp textarea") as HTMLTextAreaElement;
    const proto = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value");
    proto?.set?.call(el, "");
    el.dispatchEvent(new Event("input", { bubbles: true }));
  });

  await box.click();
  await page.keyboard.type(`Đại lý Bình Dương ${token}`);
  await page.click(".z-gs-send");
  try {
    await page.waitForFunction(
      (needle) => {
        const users = [...document.querySelectorAll(".z-gs-u")].some((el) => (el.textContent || "").includes(needle as string));
        const reply = [...document.querySelectorAll(".z-gs-ai .z-gs-body")].some((el) => (el.textContent || "").trim().length > 0);
        return users && reply;
      },
      { timeout: 20000 },
      token,
    );
  } catch (err) {
    fail("chat-send", String(err));
  }

  await page.click("[aria-label='Tìm hội thoại']");
  await page.waitForSelector(".z-gs-search input");
  await page.keyboard.type(token);
  await page.keyboard.press("Escape");
  try {
    await page.waitForFunction(() => !document.querySelector(".z-gs-search"));
  } catch (err) {
    fail("chat-search", String(err));
  }

  await page.evaluate((name) => {
    const row = [...document.querySelectorAll(".z-gs-cvw")].find((el) => (el.textContent || "").includes(name as string));
    row?.querySelector<HTMLButtonElement>(".z-gs-more")?.click();
  }, token);
  await page.waitForSelector(".z-gs-menu");
  await page.evaluate(() => {
    const button = [...document.querySelectorAll(".z-gs-menu button")].find((el) => el.textContent === "Đổi tên");
    (button as HTMLButtonElement | undefined)?.click();
  });
  await page.waitForSelector(".z-gs-rename");
  await page.click(".z-gs-rename", { clickCount: 3 });
  await page.keyboard.type(renamed);
  await page.keyboard.press("Enter");
  try {
    await page.waitForFunction((name) => document.body.innerText.includes(name as string), {}, renamed);
  } catch (err) {
    fail("chat-rename", String(err));
  }

  await page.evaluate((name) => {
    const row = [...document.querySelectorAll(".z-gs-cvw")].find((el) => (el.textContent || "").includes(name as string));
    row?.querySelector<HTMLButtonElement>(".z-gs-more")?.click();
  }, renamed);
  await page.waitForSelector(".z-gs-menu");
  const archivedClick = await page.evaluate(() => {
    const labels = [...document.querySelectorAll(".z-gs-menu button")].map((el) => (el.textContent || "").trim());
    const button = [...document.querySelectorAll(".z-gs-menu button")].find((el) => (el.textContent || "").trim() === "Lưu trữ");
    if (!button) return labels.join(" | ");
    (button as HTMLButtonElement).click();
    return "ok";
  });
  if (archivedClick !== "ok") fail("chat-archive", `menu was: ${archivedClick}`);
  try {
    await page.waitForFunction((name) => {
      const arch = (document.querySelector(".z-gs-sb")?.innerText || "").split("Đã lưu trữ")[1] || "";
      return arch.includes(name as string);
    }, {}, renamed);
    await page.evaluate((name) => {
      const rows = [...document.querySelectorAll(".z-gs-cvw")].filter((el) => (el.textContent || "").includes(name as string));
      rows[rows.length - 1]?.querySelector<HTMLButtonElement>(".z-gs-more")?.click();
    }, renamed);
    await page.waitForSelector(".z-gs-menu");
    const restored = await page.evaluate(() => {
      const button = [...document.querySelectorAll(".z-gs-menu button")].find((el) => (el.textContent || "").trim() === "Khôi phục");
      if (!button) return false;
      (button as HTMLButtonElement).click();
      return true;
    });
    if (!restored) fail("chat-restore", "Khôi phục missing");
    else {
      await page.waitForFunction((name) => {
        const arch = (document.querySelector(".z-gs-sb")?.innerText || "").split("Đã lưu trữ")[1] || "";
        const recent = (document.querySelector(".z-gs-sb")?.innerText || "").split("Đã lưu trữ")[0] || "";
        return recent.includes(name as string) && !arch.includes(name as string);
      }, {}, renamed);
    }
  } catch (err) {
    const side = await page.$eval(".z-gs-sb", (el) => el.innerText).catch(() => "");
    fail("chat-archive", `${String(err)}\n${side}`);
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
