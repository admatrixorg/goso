// GOSO Control Plane — static dist/ + reverse-proxy to gateway.
// Used by control-plane/Dockerfile (Node 22). Local dev still uses Vite.

import http from "node:http";
import { createReadStream, existsSync, statSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const dist = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "dist");
const gateway = new URL(process.env.GATEWAY_URL || "http://gateway:8080");
const port = Number(process.env.PORT || 3000);

const mime = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".json": "application/json; charset=utf-8",
  ".svg": "image/svg+xml",
  ".ico": "image/x-icon",
  ".woff2": "font/woff2",
  ".map": "application/json",
};

function shouldProxy(pathname) {
  return pathname === "/healthz" || pathname.startsWith("/api") || pathname.startsWith("/ws");
}

function proxy(req, res) {
  const opts = {
    hostname: gateway.hostname,
    port: gateway.port || (gateway.protocol === "https:" ? 443 : 80),
    path: req.url,
    method: req.method,
    headers: { ...req.headers, host: gateway.host },
  };
  const upstream = http.request(opts, (up) => {
    res.writeHead(up.statusCode || 502, up.headers);
    up.pipe(res);
  });
  upstream.on("error", (err) => {
    res.writeHead(502, { "content-type": "text/plain; charset=utf-8" });
    res.end("gateway unreachable: " + err.message);
  });
  req.pipe(upstream);
}

const server = http.createServer((req, res) => {
  const url = req.url || "/";
  const pathname = url.split("?")[0];
  if (shouldProxy(pathname)) {
    proxy(req, res);
    return;
  }
  let file = path.resolve(dist, pathname === "/" ? "index.html" : "." + pathname);
  if (!file.startsWith(dist + path.sep) && file !== dist) {
    res.writeHead(403);
    res.end();
    return;
  }
  if (!existsSync(file) || !statSync(file).isFile()) {
    file = path.join(dist, "index.html");
  }
  const type = mime[path.extname(file)] || "application/octet-stream";
  res.writeHead(200, { "content-type": type });
  createReadStream(file).pipe(res);
});

server.on("upgrade", (req, socket, head) => {
  const pathname = (req.url || "/").split("?")[0];
  if (!shouldProxy(pathname)) {
    socket.destroy();
    return;
  }
  const upstream = http.request({
    hostname: gateway.hostname,
    port: gateway.port || 80,
    path: req.url,
    method: "GET",
    headers: req.headers,
  });
  upstream.on("upgrade", (upRes, upSocket, upHead) => {
    socket.write("HTTP/1.1 101 Switching Protocols\r\n");
    for (const [k, v] of Object.entries(upRes.headers)) {
      socket.write(`${k}: ${v}\r\n`);
    }
    socket.write("\r\n");
    if (upHead.length) upSocket.write(upHead);
    if (head.length) socket.write(head);
    upSocket.pipe(socket);
    socket.pipe(upSocket);
  });
  upstream.on("error", () => socket.destroy());
  upstream.end();
});

server.listen(port, "0.0.0.0", () => {
  console.log(`GOSO control-plane listening on :${port} (gateway ${gateway.href})`);
});
