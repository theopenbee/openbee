import { test } from "node:test";
import assert from "node:assert/strict";

import {
  attachmentType,
  chunkText,
  decodeRaw,
  encodeRaw,
  planAttachments,
  stripMention,
} from "../dist/helpers.js";

test("chunkText splits at 2000", () => {
  const text = "a".repeat(4500);
  const chunks = chunkText(text);
  assert.equal(chunks.length, 3);
  for (const c of chunks) assert.ok(c.length <= 2000, `chunk of ${c.length}`);
  assert.equal(chunks.join(""), text);
});

test("chunkText prefers newlines", () => {
  const first = "x".repeat(1500) + "\n";
  const second = "y".repeat(1000);
  const chunks = chunkText(first + second);
  assert.deepEqual(chunks, [first, second]);
});

test("chunkText keeps short text and drops empty", () => {
  assert.deepEqual(chunkText("hello"), ["hello"]);
  assert.deepEqual(chunkText(""), []);
});

test("chunkText never splits a surrogate pair", () => {
  const text = "a".repeat(1999) + "😀" + "b";
  const chunks = chunkText(text);
  assert.equal(chunks.join(""), text);
  for (const c of chunks) assert.doesNotMatch(c, /^[\uDC00-\uDFFF]|[\uD800-\uDBFF]$/);
});

test("stripMention removes both forms", () => {
  assert.equal(stripMention("<@123> hello", "123"), "hello");
  assert.equal(stripMention("hey <@!123>, look", "123"), "hey , look");
  assert.equal(stripMention("<@999> hi", "123"), "<@999> hi");
});

test("attachmentType maps content types", () => {
  assert.equal(attachmentType("image/png"), "image");
  assert.equal(attachmentType("audio/ogg"), "audio");
  assert.equal(attachmentType("video/mp4"), "video");
  assert.equal(attachmentType("application/pdf"), "document");
  assert.equal(attachmentType(null), "document");
});

test("raw round-trips", () => {
  const raw = encodeRaw({ channel_id: "c1", message_id: "m1" });
  assert.deepEqual(decodeRaw(raw), { channel_id: "c1", message_id: "m1" });
  assert.equal(decodeRaw("not json"), undefined);
  assert.equal(decodeRaw(undefined), undefined);
});

test("oversized attachment becomes placeholder", () => {
  const plan = planAttachments(
    [
      { name: "a.png", size: 10 },
      { name: "big.zip", size: 100 },
    ],
    50,
  );
  assert.deepEqual(
    plan.keep.map((a) => a.name),
    ["a.png"],
  );
  assert.deepEqual(plan.notes, ["[attachment too large: big.zip]"]);
});
