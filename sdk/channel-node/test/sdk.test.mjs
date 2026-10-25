import { test } from "node:test";
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { createInterface } from "node:readline";
import { mkdtempSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const fixture = join(here, "fixtures", "echo-plugin.mjs");

function startPlugin(env = {}) {
  const child = spawn(process.execPath, [fixture], { env: { ...process.env, ...env }, stdio: ["pipe", "pipe", "pipe"] });
  const stdoutLines = [];
  const waiters = [];
  let stderr = "";
  child.stderr.on("data", (d) => (stderr += d));
  createInterface({ input: child.stdout }).on("line", (line) => {
    stdoutLines.push(line);
    const msg = JSON.parse(line);
    for (const w of [...waiters]) {
      if (w.match(msg)) {
        waiters.splice(waiters.indexOf(w), 1);
        w.resolve(msg);
      }
    }
  });
  const exited = new Promise((res) => child.on("exit", (code) => res(code)));
  let nextId = 1;
  return {
    child,
    stdoutLines,
    stderr: () => stderr,
    exited,
    write(obj) {
      child.stdin.write(JSON.stringify({ jsonrpc: "2.0", ...obj }) + "\n");
    },
    request(method, params) {
      const id = nextId++;
      this.write({ id, method, params });
      return { id, response: this.waitFor((m) => m.id === id) };
    },
    waitFor(match, ms = 5000) {
      return new Promise((resolve, reject) => {
        const timer = setTimeout(() => reject(new Error("timed out waiting for message")), ms);
        waiters.push({ match, resolve: (m) => (clearTimeout(timer), resolve(m)) });
      });
    },
  };
}

function initParams(mediaDir = tmpdir()) {
  return {
    protocol_version: 1,
    openbee_version: "test",
    channel_id: "echo",
    language: "en",
    bot_name: "Bot",
    config: { token: "x" },
    data_dir: tmpdir(),
    media_dir: mediaDir,
  };
}

async function initialized(env = {}, mediaDir) {
  const p = startPlugin(env);
  const { response } = p.request("initialize", initParams(mediaDir));
  const res = await response;
  assert.deepEqual(res.result, { protocol_version: 1 });
  return p;
}

test("initialize then send round-trip", async () => {
  const p = await initialized();
  const inbound = p.waitFor((m) => m.method === "inbound");
  const { response } = p.request("send", { session_key: "echo:c:u", chat_id: "c", session_scope: "u", proactive: false, content: "hello" });
  const res = await response;
  assert.deepEqual(res.result, { platform_message_id: "pm-1" });
  const note = await inbound;
  assert.equal(note.params.text, "hello");
  assert.equal(note.params.chat_id, "echo");
  assert.equal("id" in note, false);
  p.child.kill();
});

test("logger sends log notifications", async () => {
  const p = startPlugin();
  const log = p.waitFor((m) => m.method === "log");
  p.request("initialize", initParams());
  const note = await log;
  assert.equal(note.params.level, "info");
  assert.equal(note.params.message, "ready");
  assert.deepEqual(note.params.fields, { channel: "echo" });
  p.child.kill();
});

test("config error maps to -32001", async () => {
  const p = startPlugin({ FAIL_CONFIG: "1" });
  const { response } = p.request("initialize", initParams());
  const res = await response;
  assert.equal(res.error.code, -32001);
  assert.equal(res.error.message, "token is required");
  assert.notEqual(await p.exited, 0);
});

test("start failure maps to -32603 and exits", async () => {
  const p = startPlugin({ FAIL_START: "1" });
  const { response } = p.request("initialize", initParams());
  const res = await response;
  assert.equal(res.error.code, -32603);
  assert.equal(await p.exited, 1);
});

test("cancelRequest aborts send", async () => {
  const p = await initialized({ SLOW_SEND: "1" });
  const { id, response } = p.request("send", { session_key: "echo:c:u", chat_id: "c", session_scope: "u", proactive: false, content: "x" });
  p.write({ method: "$/cancelRequest", params: { id } });
  const res = await response;
  assert.ok(res.error, "expected an error response");
  p.child.kill();
});

test("console.log is redirected to stderr", async () => {
  const p = await initialized();
  await p.request("send", { session_key: "echo:c:u", chat_id: "c", session_scope: "u", proactive: false, content: "x" }).response;
  for (const line of p.stdoutLines) {
    assert.doesNotThrow(() => JSON.parse(line), line);
  }
  assert.match(p.stderr(), /starting echo plugin/);
  p.child.kill();
});

test("exits when stdin closes", async () => {
  const p = await initialized();
  p.child.stdin.end();
  const code = await Promise.race([p.exited, new Promise((r) => setTimeout(() => r("timeout"), 2000))]);
  assert.equal(code, 0);
  assert.match(p.stderr(), /stopped/);
});

test("exits when stdin closes before initialize", async () => {
  const p = startPlugin();
  p.child.stdin.end();
  const code = await Promise.race([p.exited, new Promise((r) => setTimeout(() => r("timeout"), 2000))]);
  assert.equal(code, 0);
});

test("shutdown stops, replies and exits", async () => {
  const p = await initialized();
  const res = await p.request("shutdown").response;
  assert.deepEqual(res.result, {});
  assert.equal(await p.exited, 0);
  assert.match(p.stderr(), /stopped/);
});

test("unknown method returns -32601", async () => {
  const p = await initialized();
  const res = await p.request("nope", {}).response;
  assert.equal(res.error.code, -32601);
  p.child.kill();
});

test("saveMedia writes into mediaDir", async () => {
  const mediaDir = mkdtempSync(join(tmpdir(), "media-"));
  const p = await initialized({ SAVE_MEDIA: "1" }, mediaDir);
  const inbound = p.waitFor((m) => m.method === "inbound");
  await p.request("send", { session_key: "echo:c:u", chat_id: "c", session_scope: "u", proactive: false, content: "x" }).response;
  const note = await inbound;
  const path = note.params.text.replace(/^saved:/, "");
  assert.equal(dirname(path), resolve(mediaDir));
  assert.match(path, /\.txt$/);
  assert.equal(readFileSync(path, "utf8"), "hello media");
  p.child.kill();
});
