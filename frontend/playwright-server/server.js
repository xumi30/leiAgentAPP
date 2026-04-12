import http from "node:http";
import { randomUUID } from "node:crypto";
import { chromium } from "playwright";

const HOST = process.env.PW_SERVER_HOST || "127.0.0.1";
const PORT = Number(process.env.PW_SERVER_PORT || "3111");
const DEFAULT_TIMEOUT_MS = Number(process.env.PW_TIMEOUT_MS || "30000");
const SCREENSHOT_DIR = process.env.PW_SCREENSHOT_DIR || "";
const DEFAULT_OBSERVE_LIMIT = Number(process.env.PW_OBSERVE_LIMIT || "8");

/** @type {Map<string, { browser: import('playwright').Browser, context: import('playwright').BrowserContext, page: import('playwright').Page }>} */
const sessions = new Map();

function json(res, statusCode, data) {
  const body = JSON.stringify(data ?? null);
  res.writeHead(statusCode, {
    "content-type": "application/json; charset=utf-8",
    "content-length": Buffer.byteLength(body),
  });
  res.end(body);
}

async function readBody(req) {
  const chunks = [];
  for await (const c of req) chunks.push(c);
  const raw = Buffer.concat(chunks).toString("utf8").trim();
  if (!raw) return {};
  try {
    return JSON.parse(raw);
  } catch {
    const err = new Error("invalid json body");
    err.code = "invalid_json";
    throw err;
  }
}

function notFound(res) {
  json(res, 404, { ok: false, error: { code: "not_found", message: "not found" } });
}

function methodNotAllowed(res) {
  json(res, 405, { ok: false, error: { code: "method_not_allowed", message: "method not allowed" } });
}

function getSession(sessionId) {
  if (!sessionId || typeof sessionId !== "string") {
    const err = new Error("sessionId is required");
    err.code = "bad_request";
    throw err;
  }
  const s = sessions.get(sessionId);
  if (!s) {
    const err = new Error(`unknown sessionId: ${sessionId}`);
    err.code = "unknown_session";
    throw err;
  }
  return s;
}

async function resolveTextLocator(page, text, exact = true, preferredRole = "") {
  const roles = preferredRole
    ? [preferredRole]
    : ["link", "button", "tab", "menuitem"];

  for (const role of roles) {
    const locator = page.getByRole(role, { name: text, exact });
    const count = await locator.count();
    if (count > 0) {
      return { locator: locator.first(), matchedBy: `role:${role}` };
    }
  }

  const textLocator = page.getByText(text, { exact });
  const textCount = await textLocator.count();
  if (textCount > 0) {
    return { locator: textLocator.first(), matchedBy: "text" };
  }

  return { locator: null, matchedBy: "" };
}

async function collectLinks(page, limit = 20) {
  return page.evaluate((max) => {
    const anchors = Array.from(document.querySelectorAll("a"));
    return anchors
      .map((a) => ({
        text: (a.textContent || "").trim(),
        title: (a.getAttribute("title") || "").trim(),
        href: a.href || "",
        ariaLabel: (a.getAttribute("aria-label") || "").trim(),
      }))
      .filter((item) => item.text || item.title || item.ariaLabel || item.href)
      .slice(0, max);
  }, limit);
}

async function collectInputs(page, limit = 12) {
  return page.evaluate((max) => {
    const elements = Array.from(document.querySelectorAll("input, textarea, select"));
    return elements
      .map((el) => ({
        tag: el.tagName.toLowerCase(),
        type: (el.getAttribute("type") || "").trim(),
        name: (el.getAttribute("name") || "").trim(),
        id: (el.getAttribute("id") || "").trim(),
        placeholder: (el.getAttribute("placeholder") || "").trim(),
        ariaLabel: (el.getAttribute("aria-label") || "").trim(),
        value: el.tagName === "SELECT" ? "" : String(el.value || "").trim().slice(0, 80),
      }))
      .filter((item) => item.name || item.id || item.placeholder || item.ariaLabel || item.type)
      .slice(0, max);
  }, limit);
}

async function collectButtons(page, limit = 10) {
  return page.evaluate((max) => {
    const elements = Array.from(document.querySelectorAll("button, [role='button'], input[type='submit']"));
    return elements
      .map((el) => ({
        text: (el.textContent || el.getAttribute("value") || "").trim(),
        id: (el.getAttribute("id") || "").trim(),
        ariaLabel: (el.getAttribute("aria-label") || "").trim(),
        title: (el.getAttribute("title") || "").trim(),
      }))
      .filter((item) => item.text || item.id || item.ariaLabel || item.title)
      .slice(0, max);
  }, limit);
}

async function collectHeadings(page, limit = 6) {
  return page.evaluate((max) => {
    const elements = Array.from(document.querySelectorAll("h1, h2, h3, [role='heading']"));
    return elements
      .map((el) => (el.textContent || "").trim())
      .filter(Boolean)
      .slice(0, max);
  }, limit);
}

async function observePage(page, limit = DEFAULT_OBSERVE_LIMIT) {
  const safeLimit = Number.isFinite(limit) && limit > 0 ? Math.min(limit, 20) : DEFAULT_OBSERVE_LIMIT;
  const [title, url, headings, links, inputs, buttons] = await Promise.all([
    page.title().catch(() => ""),
    Promise.resolve(page.url()),
    collectHeadings(page, Math.min(safeLimit, 6)).catch(() => []),
    collectLinks(page, safeLimit).catch(() => []),
    collectInputs(page, safeLimit).catch(() => []),
    collectButtons(page, safeLimit).catch(() => []),
  ]);

  return { title, url, headings, links, inputs, buttons };
}

async function maybeAttachObservation(page, params, result, defaultObserve = true) {
  const observe = params.observe !== undefined ? params.observe !== false : defaultObserve;
  if (!observe) return result;
  const observeLimit = typeof params.observeLimit === "number" ? params.observeLimit : DEFAULT_OBSERVE_LIMIT;
  return {
    ...result,
    observation: await observePage(page, observeLimit),
  };
}

async function handleCreateSession(req, res) {
  const body = await readBody(req);
  const headless = body.headless !== false;
  const viewport = body.viewport && typeof body.viewport === "object" ? body.viewport : undefined;
  const userAgent = typeof body.userAgent === "string" ? body.userAgent : undefined;
  const locale = typeof body.locale === "string" ? body.locale : undefined;

  const browser = await chromium.launch({ headless });
  const context = await browser.newContext({
    viewport: viewport && typeof viewport.width === "number" && typeof viewport.height === "number" ? viewport : undefined,
    userAgent,
    locale,
  });
  const page = await context.newPage();

  const sessionId = randomUUID();
  sessions.set(sessionId, { browser, context, page });
  json(res, 200, { ok: true, sessionId });
}

async function handleDeleteSession(req, res, sessionId) {
  const s = sessions.get(sessionId);
  if (!s) return json(res, 200, { ok: true, sessionId, closed: false, reason: "unknown_session" });
  sessions.delete(sessionId);
  await Promise.allSettled([s.page.close(), s.context.close(), s.browser.close()]);
  json(res, 200, { ok: true, sessionId, closed: true });
}

async function handleAction(req, res) {
  const body = await readBody(req);
  const sessionId = body.sessionId;
  const action = body.action;
  const params = body.params && typeof body.params === "object" ? body.params : {};
  const timeoutMs = typeof body.timeoutMs === "number" ? body.timeoutMs : DEFAULT_TIMEOUT_MS;

  if (typeof action !== "string") {
    return json(res, 400, { ok: false, error: { code: "bad_request", message: "action must be a string" } });
  }

  const { page } = getSession(sessionId);
  page.setDefaultTimeout(timeoutMs);

  const startedAt = Date.now();
  try {
    /** @type {any} */
    let result = null;

    switch (action) {
      case "goto": {
        const url = params.url;
        if (typeof url !== "string") throw Object.assign(new Error("params.url must be a string"), { code: "bad_request" });
        const waitUntil = typeof params.waitUntil === "string" ? params.waitUntil : "load";
        const resp = await page.goto(url, { waitUntil });
        result = await maybeAttachObservation(page, params, { url: page.url(), status: resp?.status?.() ?? null });
        break;
      }
      case "click": {
        const selector = params.selector;
        if (typeof selector !== "string") throw Object.assign(new Error("params.selector must be a string"), { code: "bad_request" });
        await page.click(selector);
        result = await maybeAttachObservation(page, params, { clicked: selector });
        break;
      }
      case "click_text": {
        const text = params.text;
        if (typeof text !== "string") throw Object.assign(new Error("params.text must be a string"), { code: "bad_request" });
        const exact = params.exact !== false;
        const role = typeof params.role === "string" ? params.role : "";
        const { locator, matchedBy } = await resolveTextLocator(page, text, exact, role);
        if (!locator) {
          throw Object.assign(new Error(`no element found by text: ${text}`), { code: "not_found" });
        }
        await locator.click();
        result = await maybeAttachObservation(page, params, { clickedText: text, matchedBy });
        break;
      }
      case "fill": {
        const selector = params.selector;
        const value = params.value;
        if (typeof selector !== "string") throw Object.assign(new Error("params.selector must be a string"), { code: "bad_request" });
        if (typeof value !== "string") throw Object.assign(new Error("params.value must be a string"), { code: "bad_request" });
        await page.fill(selector, value);
        result = await maybeAttachObservation(page, params, { filled: selector }, false);
        break;
      }
      case "press": {
        const selector = params.selector;
        const key = params.key;
        if (typeof selector !== "string") throw Object.assign(new Error("params.selector must be a string"), { code: "bad_request" });
        if (typeof key !== "string") throw Object.assign(new Error("params.key must be a string"), { code: "bad_request" });
        await page.press(selector, key);
        result = await maybeAttachObservation(page, params, { pressed: key, selector });
        break;
      }
      case "wait_for_selector": {
        const selector = params.selector;
        if (typeof selector !== "string") throw Object.assign(new Error("params.selector must be a string"), { code: "bad_request" });
        await page.waitForSelector(selector);
        result = await maybeAttachObservation(page, params, { found: selector }, false);
        break;
      }
      case "list_links": {
        const limit = typeof params.limit === "number" ? params.limit : 20;
        result = {
          url: page.url(),
          title: await page.title().catch(() => ""),
          links: await collectLinks(page, limit),
        };
        break;
      }
      case "list_inputs": {
        const limit = typeof params.limit === "number" ? params.limit : 12;
        result = {
          url: page.url(),
          title: await page.title().catch(() => ""),
          inputs: await collectInputs(page, limit),
        };
        break;
      }
      case "observe": {
        const limit = typeof params.limit === "number" ? params.limit : DEFAULT_OBSERVE_LIMIT;
        result = await observePage(page, limit);
        break;
      }
      case "evaluate": {
        const expression = params.expression;
        if (typeof expression !== "string") throw Object.assign(new Error("params.expression must be a string"), { code: "bad_request" });
        // expression is run in page context. Keep it explicit to avoid confusion.
        // eslint-disable-next-line no-new-func
        const fn = new Function("return (" + expression + ")")();
        result = await page.evaluate(fn);
        break;
      }
      case "screenshot": {
        const fullPage = params.fullPage !== false;
        const path = typeof params.path === "string" ? params.path : "";
        const screenshotPath = path || (SCREENSHOT_DIR ? `${SCREENSHOT_DIR}/pw_${sessionId}_${Date.now()}.png` : "");
        const buf = await page.screenshot({ fullPage, path: screenshotPath || undefined });
        result = { bytesBase64: buf.toString("base64"), path: screenshotPath || null };
        break;
      }
      case "content": {
        result = { html: await page.content() };
        break;
      }
      case "text": {
        const selector = params.selector;
        if (typeof selector !== "string") throw Object.assign(new Error("params.selector must be a string"), { code: "bad_request" });
        const txt = await page.textContent(selector);
        result = { selector, text: txt ?? "" };
        break;
      }
      case "url": {
        result = { url: page.url() };
        break;
      }
      default:
        return json(res, 400, { ok: false, error: { code: "unknown_action", message: `unknown action: ${action}` } });
    }

    json(res, 200, {
      ok: true,
      sessionId,
      action,
      tookMs: Date.now() - startedAt,
      result,
    });
  } catch (e) {
    const pageInfo = await (async () => {
      try {
        return {
          url: page.url(),
          title: await page.title(),
        };
      } catch {
        return { url: "", title: "" };
      }
    })();
    json(res, 500, {
      ok: false,
      sessionId,
      action,
      tookMs: Date.now() - startedAt,
      page: pageInfo,
      error: {
        code: e?.code || "execution_failed",
        message: e?.message || String(e),
      },
    });
  }
}

const server = http.createServer(async (req, res) => {
  try {
    const url = new URL(req.url || "/", `http://${req.headers.host || `${HOST}:${PORT}`}`);
    if (req.method === "GET" && url.pathname === "/health") {
      return json(res, 200, { ok: true, sessions: sessions.size });
    }

    if (req.method === "POST" && url.pathname === "/sessions") {
      return await handleCreateSession(req, res);
    }

    if (url.pathname.startsWith("/sessions/")) {
      const sessionId = decodeURIComponent(url.pathname.slice("/sessions/".length));
      if (req.method === "DELETE") return await handleDeleteSession(req, res, sessionId);
      return methodNotAllowed(res);
    }

    if (req.method === "POST" && url.pathname === "/actions") {
      return await handleAction(req, res);
    }

    return notFound(res);
  } catch (e) {
    json(res, 500, { ok: false, error: { code: e?.code || "server_error", message: e?.message || String(e) } });
  }
});

server.listen(PORT, HOST, () => {
  // Keep output minimal (can be captured by calling process).
  console.log(`playwright server listening on http://${HOST}:${PORT}`);
});
