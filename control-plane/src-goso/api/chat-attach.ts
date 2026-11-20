/** Composer chips. Binary files are named only — the gateway does not read them yet. */

export const ATTACH_MAX_BYTES = 200 * 1024 * 1024;
export const LONG_PASTE_CHARS = 1200;

export type FileChip = {
  id: string;
  kind: "file";
  name: string;
  typeLabel: string;
  size: number;
  image: boolean;
};

export type TextChip = {
  id: string;
  kind: "text";
  name: string;
  typeLabel: string;
  text: string;
};

export type Chip = FileChip | TextChip;

export type AttachResult = {
  chips: Chip[];
  errors: string[];
  insert: string;
  handled: boolean;
};

function typeLabel(name: string, mime: string): string {
  const ext = name.includes(".") ? (name.split(".").pop() || "").replace(/[^a-z0-9]/gi, "").slice(0, 8) : "";
  if (ext) return ext.toUpperCase();
  if (mime.startsWith("image/")) return "IMAGE";
  return "FILE";
}

export function fileChips(files: File[], id: () => string): { chips: FileChip[]; errors: string[] } {
  const chips: FileChip[] = [];
  const errors: string[] = [];
  for (const file of files) {
    if (file.size > ATTACH_MAX_BYTES) {
      errors.push(file.name || "file");
      continue;
    }
    chips.push({
      id: id(),
      kind: "file",
      name: file.name || "file",
      typeLabel: typeLabel(file.name || "", file.type || ""),
      size: file.size,
      image: (file.type || "").startsWith("image/"),
    });
  }
  return { chips, errors };
}

export function htmlTableToMarkdown(html: string): string | null {
  if (!/<table[\s>]/i.test(html)) return null;
  const rows = [...html.matchAll(/<tr[\s\S]*?<\/tr>/gi)]
    .map((row) =>
      [...row[0].matchAll(/<t[dh][^>]*>([\s\S]*?)<\/t[dh]>/gi)].map((cell) =>
        cell[1].replace(/<[^>]+>/g, " ").replace(/&nbsp;/g, " ").replace(/&amp;/g, "&").replace(/\s+/g, " ").trim(),
      ),
    )
    .filter((cells) => cells.length > 0);
  if (!rows.length) return null;
  const width = Math.max(...rows.map((row) => row.length));
  const norm = rows.map((row) => {
    const next = row.slice();
    while (next.length < width) next.push("");
    return next;
  });
  const fmt = (row: string[]) => `| ${row.join(" | ")} |`;
  const head = norm[0];
  const rule = head.map(() => "---");
  return [fmt(head), fmt(rule), ...norm.slice(1).map(fmt)].join("\n");
}

function isLoneUrl(text: string): boolean {
  return /^https?:\/\/\S+$/i.test(text.trim());
}

export function planPaste(input: { files: File[]; html: string; text: string }, id: () => string): AttachResult {
  if (input.files.length) {
    const got = fileChips(input.files, id);
    return { chips: got.chips, errors: got.errors, insert: "", handled: got.chips.length > 0 || got.errors.length > 0 };
  }
  const table = htmlTableToMarkdown(input.html || "");
  if (table) {
    return {
      chips: [{ id: id(), kind: "text", name: "Bảng", typeLabel: "MD", text: table }],
      errors: [],
      insert: "",
      handled: true,
    };
  }
  const text = input.text || "";
  if (text.trim().length > LONG_PASTE_CHARS) {
    const name = text.trim().slice(0, 42);
    return {
      chips: [{ id: id(), kind: "text", name, typeLabel: "TXT", text: text.trim() }],
      errors: [],
      insert: "",
      handled: true,
    };
  }
  if (isLoneUrl(text)) {
    const url = text.trim();
    return {
      chips: [{ id: id(), kind: "text", name: url, typeLabel: "URL", text: url }],
      errors: [],
      insert: "",
      handled: true,
    };
  }
  return { chips: [], errors: [], insert: text, handled: false };
}

/** Text chips are included. File chips are a filename note — contents are not read. */
export function messageFromComposer(text: string, chips: Chip[], unread: (name: string) => string): string {
  const parts: string[] = [];
  const body = text.trim();
  if (body) parts.push(body);
  for (const chip of chips) {
    if (chip.kind === "text") {
      const value = chip.text.trim();
      if (value) parts.push(value);
    } else {
      parts.push(unread(chip.name));
    }
  }
  return parts.join("\n\n").trim();
}
