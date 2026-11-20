import assert from "node:assert/strict";
import test from "node:test";
import { ATTACH_MAX_BYTES, fileChips, htmlTableToMarkdown, messageFromComposer, planPaste } from "./chat-attach.ts";

let n = 0;
const id = () => `c${(n += 1)}`;

test("file chips keep a name and refuse anything over 200 MB", () => {
  n = 0;
  const ok = new File(["hi"], "Hop-dong-mau.pdf", { type: "application/pdf" });
  const big = new File([new Uint8Array(8)], "qua-lon.zip", { type: "application/zip" });
  Object.defineProperty(big, "size", { value: ATTACH_MAX_BYTES + 1 });
  const got = fileChips([ok, big], id);
  assert.equal(got.chips.length, 1);
  assert.equal(got.chips[0].name, "Hop-dong-mau.pdf");
  assert.equal(got.chips[0].typeLabel, "PDF");
  assert.equal(got.chips[0].image, false);
  assert.deepEqual(got.errors, ["qua-lon.zip"]);
});

test("an html table becomes markdown and a long paste becomes one chip", () => {
  n = 0;
  const md = htmlTableToMarkdown("<table><tr><td>Đại lý</td><td>Nợ</td></tr><tr><td>Bình Dương</td><td>52</td></tr></table>");
  assert.equal(md, "| Đại lý | Nợ |\n| --- | --- |\n| Bình Dương | 52 |");
  const table = planPaste({ files: [], html: "<table><tr><th>A</th></tr></table>", text: "" }, id);
  assert.equal(table.handled, true);
  assert.equal(table.chips[0].kind, "text");
  assert.match(table.chips[0].kind === "text" ? table.chips[0].text : "", /\| A \|/);
  const long = planPaste({ files: [], html: "", text: "a".repeat(1201) }, id);
  assert.equal(long.chips.length, 1);
  assert.equal(long.insert, "");
  const plain = planPaste({ files: [], html: "", text: "hỏi công nợ" }, id);
  assert.equal(plain.handled, false);
  assert.equal(plain.insert, "hỏi công nợ");
});

test("send text includes a table and names a file that was not read", () => {
  const body = messageFromComposer(
    "Đại lý Bình Dương còn nợ bao nhiêu?",
    [
      { id: "1", kind: "text", name: "Bảng", typeLabel: "MD", text: "| Đại lý | Nợ |" },
      { id: "2", kind: "file", name: "Hop-dong-mau.pdf", typeLabel: "PDF", size: 12, image: false },
    ],
    (name) => `Tệp đính kèm (chưa đọc): ${name}`,
  );
  assert.match(body, /Đại lý Bình Dương/);
  assert.match(body, /\| Đại lý \| Nợ \|/);
  assert.match(body, /chưa đọc\): Hop-dong-mau\.pdf/);
  assert.doesNotMatch(body, /Đã đọc/);
});
