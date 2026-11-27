#!/usr/bin/env python3
"""Sinh bảng việc từ kanban/data.json (nguồn sự thật duy nhất).

    python3 kanban/render.py            sinh README.md, chi-tiet.md, web/data.js, web/data.json
    python3 kanban/render.py --tom-tat  in vài dòng cho đầu phiên: số thẻ theo cột, điểm nghẽn,
                                        PR mở chưa có thẻ, thẻ trỏ PR đã đóng, bảng lệch data.json

Khuôn lấy từ admatrixmdp/admatrix-process (scripts/render_readme.py). Không phụ thuộc gói ngoài.
"""

from __future__ import annotations

import json
import os
import re
import shutil
import subprocess
import sys
from html import escape
from pathlib import Path

HERE = Path(__file__).resolve().parent
DATA_PATH = HERE / "data.json"
README_PATH = HERE / "README.md"
DETAIL_PATH = HERE / "chi-tiet.md"
WEB_DIR = HERE / "web"

COLUMN_EMOJI = {"todo": "📋", "doing": "🚧", "review": "🔎", "blocked": "⛔", "done": "✅"}
HEADER_BG = {"todo": "#f5f5f4", "doing": "#eff6ff", "review": "#fff7ed", "blocked": "#fff1f2", "done": "#ecfdf5"}
GENERATED_NOTE = "Sinh bởi kanban/render.py từ kanban/data.json. Đừng sửa tay."
PR_URL = re.compile(r"/pull/(\d+)")


def load_data() -> dict:
    data = json.loads(DATA_PATH.read_text(encoding="utf-8"))
    cards = data["cards"]
    if not cards:
        raise SystemExit("Cần ít nhất 1 thẻ")
    ids = [card["id"] for card in cards]
    dup = sorted({i for i in ids if ids.count(i) > 1})
    if dup:
        raise SystemExit(f"Trùng id thẻ: {dup}")
    known = {col["id"] for col in data["columns"]}
    if known != set(COLUMN_EMOJI):
        raise SystemExit(f"Cột phải đúng 5 id {sorted(COLUMN_EMOJI)}, đang có {sorted(known)}")
    unknown = [card["id"] for card in cards if card["column"] not in known]
    if unknown:
        raise SystemExit(f"Thẻ trỏ vào cột lạ: {unknown}")
    missing = [card["id"] for card in cards if not card.get("id") or not card.get("title") or not card.get("url") or not card.get("owner")]
    if missing:
        raise SystemExit(f"Thẻ thiếu id/title/url/owner (url trỏ PR, issue hoặc tệp spec/sổ chờ PO): {missing}")
    streams = {s["id"] for s in data.get("streams") or []}
    bad_stream = [card["id"] for card in cards if card.get("stream") and card["stream"] not in streams]
    if bad_stream:
        raise SystemExit(f"Thẻ trỏ vào stream chưa khai trong 'streams': {bad_stream}")
    return data


def cards_in(data: dict, column_id: str) -> list[dict]:
    return [card for card in data["cards"] if card["column"] == column_id]


def wip_label(column: dict, count: int) -> str:
    if column.get("wip_limit") is None:
        return str(count)
    label = f"{count}/{column['wip_limit']}"
    return f"{label} — vượt WIP" if count > column["wip_limit"] else label


def over_wip(column: dict, count: int) -> bool:
    limit = column.get("wip_limit")
    return limit is not None and count > limit


def stream_label(data: dict, stream_id: str | None) -> str:
    for stream in data.get("streams") or []:
        if stream["id"] == stream_id:
            return stream["label"]
    return stream_id or ""


def col_by_id(data: dict) -> dict:
    return {col["id"]: col for col in data["columns"]}


def render_card_html(card: dict) -> str:
    title_id = f'<strong><a href="{escape(card["url"], quote=True)}">{escape(card["id"])}</a></strong>'
    if card.get("spec"):
        title_id += f' · SPEC {escape(str(card["spec"]))}'
    if card.get("bottleneck"):
        title_id += " ⚠️"
    sub_parts = [escape(card["owner"])]
    status = card.get("status") or ""
    if status:
        sub_parts.append(escape(status))
    mergeable = card.get("mergeable")
    if mergeable and mergeable not in status:
        sub_parts.append(escape(mergeable))
    if card.get("head"):
        sub_parts.append(f'<code>{escape(card["head"])}</code>')
    bits = ["<p>", title_id + "<br>", f"{escape(card['title'])}<br>", "<sub>", " · ".join(sub_parts), "</sub>"]
    if card.get("next_step"):
        bits.append(f'<br><sub>➡️ {escape(card["next_step"])}</sub>')
    bits.append("</p>")
    return "".join(bits)


def bottleneck_label(data: dict) -> str:
    review = col_by_id(data)["review"]
    n = len(cards_in(data, "review"))
    if n > review["wip_limit"]:
        return f"In Review {n}/{review['wip_limit']} — vượt WIP"
    if n > 0:
        return f"In Review {n}/{review['wip_limit']}"
    return "Không còn PR kẹt ở review"


def render_readme(data: dict) -> str:
    project = data["project"]
    cols = col_by_id(data)
    counts = {col["id"]: len(cards_in(data, col["id"])) for col in data["columns"]}

    header_cells, body_cells = [], []
    for col in data["columns"]:
        label = f'{COLUMN_EMOJI[col["id"]]} {escape(col["label"])}<br><sub>{escape(wip_label(col, counts[col["id"]]))}</sub>'
        header_cells.append(f'<th align="left" width="20%" bgcolor="{HEADER_BG[col["id"]]}">{label}</th>')
        cards_html = []
        current_group: object = object()
        for card in cards_in(data, col["id"]):
            group = card.get("group")
            if group and group != current_group:
                current_group = group
                cards_html.append(f'<p><sub><strong>{escape(card.get("group_label") or group)}</strong></sub></p>')
            elif not group:
                current_group = None
            cards_html.append(render_card_html(card))
        body_cells.append('<td valign="top" width="20%">\n' + "\n".join(cards_html) + "\n</td>")

    mermaid = f"""```mermaid
flowchart LR
  TODO[\"To Do\\n{counts['todo']} thẻ\"] --> DOING[\"Doing\\n{counts['doing']} / WIP {cols['doing']['wip_limit']}\"]
  DOING --> REVIEW[\"In Review\\n{counts['review']} / WIP {cols['review']['wip_limit']}\"]
  REVIEW --> BLOCKED[\"Blocked / Chờ duyệt\\n{counts['blocked']} thẻ\"]
  REVIEW --> DONE[\"Done\\n{counts['done']} thẻ\"]
  BLOCKED --> DONE
  style REVIEW fill:#fff7ed,stroke:#d97706,stroke-width:2px
  style BLOCKED fill:#fff1f2,stroke:#e11d48
  style DONE fill:#ecfdf5,stroke:#059669
  style DOING fill:#eff6ff,stroke:#2563eb
  style TODO fill:#f5f5f4,stroke:#78716c
```"""

    table = (
        "<table>\n<thead>\n<tr>\n" + "\n".join(header_cells)
        + '\n</tr>\n</thead>\n<tbody>\n<tr valign="top">\n' + "\n".join(body_cells)
        + "\n</tr>\n</tbody>\n</table>"
    )
    ledgers = "".join(f"\n- **{escape(k)}:** {v}" for k, v in (project.get("ledgers") or {}).items())

    return f"""<!-- {GENERATED_NOTE} -->
# Bảng việc — {escape(project['name'])}

Bảng Kanban của **{escape(project['repo'])}**, 5 cột từ **To Do → Done**. Agent đọc bảng này đầu phiên để biết repo đang có gì và tới đâu.

- **Cập nhật:** `{escape(project['updated_at'])}` · **`main`:** [`{escape(project['main_commit'])}`]({project['main_url']})
- **Nguồn dữ liệu duy nhất:** [`kanban/data.json`](data.json)
- **Bản web** (lọc owner/stream, điểm nghẽn, sáng/tối): mở [`kanban/web/index.html`](web/index.html) — mở thẳng tệp cũng chạy (dữ liệu nhúng trong `web/data.js`).
- **Bản chi tiết theo cột:** [`kanban/chi-tiet.md`](chi-tiet.md){ledgers}

## Chỉ số tổng quan

| `main` | PR đã gộp 7 ngày | PR đang mở | Doing / WIP | Điểm nghẽn |
| --- | --- | --- | --- | --- |
| [`{escape(project['main_commit'])}`]({project['main_url']}) | `{escape(str(project['prs_merged']))}` | `{project['prs_open']}` | `{wip_label(cols['doing'], counts['doing'])}` | **{escape(bottleneck_label(data))}** |

> **Điểm nghẽn số 1:** {escape(project['bottleneck_summary'])}

## Luồng Kanban

{mermaid}

## Bảng Kanban

{table}

<details>
<summary>Cách cập nhật bảng</summary>

1. Sửa nguồn sự thật [`kanban/data.json`](data.json) — không sửa tay README này.
   - Mở PR: thêm thẻ (`kind: "pr"`, `url` trỏ PR) vào cột `review`, hoặc `doing` nếu còn nháp.
   - Sắp gộp PR: commit cuối của chính PR đó chuyển thẻ sang `done`, ghi `status` (để main không trễ một nhịp). PR đóng không gộp: chuyển `done`, ghi lý do. Kẹt chờ PO: cột `blocked`, `next_step` ghi đang chờ gì.
   - Bắt đầu việc chưa có PR: thẻ cột `doing` (`kind: "issue"`), `head` là nhánh.
2. Chạy `python3 kanban/render.py` để sinh lại `kanban/README.md`, `kanban/chi-tiet.md`, `kanban/web/data.js`, `kanban/web/data.json`.
3. Commit cùng PR đang làm (không mở PR riêng chỉ để sửa bảng).
4. `python3 kanban/render.py --tom-tat` liệt kê PR mở chưa có thẻ và thẻ trỏ PR đã đóng.

</details>
"""


def render_detail_md(data: dict) -> str:
    project = data["project"]
    lines = [
        f"<!-- {GENERATED_NOTE} -->",
        f"# Kanban Board: {project['name']}",
        "",
        f"- **Repo:** [{project['repo']}]({project['repo_url']}) · **Cập nhật:** `{project['updated_at']}` · **`main`:** [`{project['main_commit']}`]({project['main_url']})",
        f"- **Tổng:** `{project['prs_merged']}` PR gộp trong 7 ngày, `{project['prs_open']}` PR đang mở",
        f"- **Điểm nghẽn số 1:** {project['bottleneck_summary']}",
        "- **Nguồn dữ liệu:** [`data.json`](data.json) · **Bảng web:** [`web/index.html`](web/index.html)",
        "",
    ]
    for index, col in enumerate(data["columns"], start=1):
        cards = cards_in(data, col["id"])
        heading = f"## {index}. {COLUMN_EMOJI[col['id']]} {col['label']} (`{wip_label(col, len(cards))}`)"
        if over_wip(col, len(cards)):
            heading += " — ĐIỂM NGHẼN"
        lines.append(heading)
        if not cards:
            lines += ["- _Không có thẻ._", ""]
            continue
        groups: list[tuple[str | None, str | None, list[dict]]] = []
        for card in cards:
            key = card.get("group")
            if not groups or groups[-1][0] != key:
                groups.append((key, card.get("group_label"), [card]))
            else:
                groups[-1][2].append(card)
        for group_key, group_label, group_cards in groups:
            if group_key and group_label:
                lines.append(f"**{group_label} (`{len(group_cards)}`):**")
            for card in group_cards:
                extras = []
                if card.get("spec"):
                    extras.append(f"SPEC {card['spec']}")
                if card.get("head"):
                    extras.append(f"`{card['head']}`")
                if card.get("mergeable"):
                    extras.append(f"`{card['mergeable']}`")
                if card.get("status"):
                    extras.append(card["status"])
                extra = f" ({', '.join(extras)})" if extras else ""
                nxt = f" — ➡️ {card['next_step']}" if card.get("next_step") else ""
                note = f"\n  - {card['note']}" if card.get("note") else ""
                flag = " ⚠️" if card.get("bottleneck") else ""
                lines.append(f"- **[{card['id']}]({card['url']})** {card['title']}{extra} — *Owner: {card['owner']}*{nxt}{flag}{note}")
            lines.append("")
    lines += [
        "## Bảng kiểm tra nhanh",
        "",
        "| ID | Cột | Owner | Stream | Trạng thái | Mergeable | Head | Điểm nghẽn | Bước tiếp theo |",
        "| --- | --- | --- | --- | --- | --- | --- | --- | --- |",
    ]
    col_label = {col["id"]: col["label"] for col in data["columns"]}
    for col in data["columns"]:
        for card in cards_in(data, col["id"]):
            cells = [
                f"[{card['id']}]({card['url']})",
                col_label[card["column"]],
                card["owner"],
                stream_label(data, card.get("stream")),
                card.get("status") or "",
                card.get("mergeable") or "",
                f"`{card['head']}`" if card.get("head") else "",
                "Có" if card.get("bottleneck") else "",
                card.get("next_step") or "",
            ]
            lines.append("| " + " | ".join(c.replace("|", "\\|") for c in cells) + " |")
    lines.append("")
    return "\n".join(lines)


def web_payloads(data: dict) -> tuple[str, str]:
    payload = json.dumps(data, ensure_ascii=False, indent=2) + "\n"
    return payload, "/* " + GENERATED_NOTE + " */\nwindow.KANBAN_DATA = " + payload.rstrip() + ";\n"


def outputs(data: dict) -> dict[Path, str]:
    data_json, data_js = web_payloads(data)
    return {
        README_PATH: render_readme(data),
        DETAIL_PATH: render_detail_md(data),
        WEB_DIR / "data.json": data_json,
        WEB_DIR / "data.js": data_js,
    }


def card_pr_numbers(card: dict) -> set[int]:
    nums = set(card.get("prs") or [])
    m = PR_URL.search(card.get("url") or "")
    if m and card.get("kind") == "pr":
        nums.add(int(m.group(1)))
    return nums


def gh_open_prs(repo: str) -> list[dict] | None:
    """PR đang mở theo gh; None khi chạy offline (KANBAN_OFFLINE=1) hoặc gh lỗi. KANBAN_GH chọn lệnh gh khác."""
    if os.environ.get("KANBAN_OFFLINE") == "1":
        return None
    gh = os.environ.get("KANBAN_GH") or shutil.which("gh")
    if not gh:
        return None
    try:
        out = subprocess.run(
            [gh, "pr", "list", "-R", repo, "--state", "open", "--limit", "200", "--json", "number,title,author"],
            capture_output=True, text=True, timeout=20, check=True,
        ).stdout
        return json.loads(out)
    except (subprocess.SubprocessError, ValueError, OSError):
        return None


def summary(data: dict) -> int:
    project = data["project"]
    cols = col_by_id(data)
    parts = []
    for col in data["columns"]:
        n = len(cards_in(data, col["id"]))
        parts.append(f"{col['label']} {wip_label(col, n)}")
    print(f"Bảng việc ({project['updated_at']}): " + " · ".join(parts))
    print(f"  Điểm nghẽn: {project['bottleneck_summary']}")
    for card in cards_in(data, "doing") + cards_in(data, "blocked"):
        print(f"  {cols[card['column']]['label']}: {card['id']} — {card['title']}" + (f" ➡️ {card['next_step']}" if card.get("next_step") else ""))

    stale = [p.name for p, text in outputs(data).items() if not p.is_file() or p.read_text(encoding="utf-8") != text]
    if stale:
        print(f"  ⚠️ Bảng lệch data.json ({', '.join(stale)}): chạy python3 kanban/render.py")

    open_prs = gh_open_prs(project["repo"])
    if open_prs is None:
        print("  (không đối chiếu PR: offline hoặc gh lỗi)")
        return 0
    open_nums = {p["number"] for p in open_prs}
    tracked: dict[int, dict] = {}
    for card in data["cards"]:
        for n in card_pr_numbers(card):
            tracked[n] = card
    missing = [p for p in open_prs if p["number"] not in tracked]
    if missing:
        print(f"  ⚠️ {len(missing)} PR mở chưa có thẻ: " + ", ".join(f"#{p['number']} {p['title'][:50]}" for p in missing[:8]))
    closed = sorted(n for n, card in tracked.items() if n not in open_nums and card["column"] != "done")
    if closed:
        print(f"  ⚠️ Thẻ trỏ PR không còn mở mà chưa sang Done: " + ", ".join(f"#{n} ({tracked[n]['id']})" for n in closed))
    if not missing and not closed:
        print(f"  Khớp GitHub: {len(open_nums)} PR mở đều có thẻ.")
    return 0


def main(argv: list[str]) -> int:
    if not DATA_PATH.is_file():
        print(f"Thiếu tệp dữ liệu: {DATA_PATH}", file=sys.stderr)
        return 1
    data = load_data()
    if "--tom-tat" in argv:
        return summary(data)
    for path, text in outputs(data).items():
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text, encoding="utf-8")
        print(f"Đã ghi {path.relative_to(HERE.parent)}")
    node = shutil.which("node")
    if node:
        for script in (WEB_DIR / "data.js", WEB_DIR / "app.js"):
            if script.is_file():
                subprocess.run([node, "--check", str(script)], check=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
