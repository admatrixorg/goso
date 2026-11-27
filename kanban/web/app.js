(function () {
  "use strict";

  var PALETTE = ["#0f766e", "#7c3aed", "#2563eb", "#c2410c", "#0369a1", "#4f46e5", "#475569", "#b45309", "#be185d", "#15803d", "#e11d48"];
  var STREAM_COLORS = {};

  function setStreamColors(streams) {
    (streams || []).forEach(function (stream, i) {
      STREAM_COLORS[stream.id] = stream.color || PALETTE[i % PALETTE.length];
    });
  }

  var COL_COLORS = {
    todo: "var(--todo)",
    doing: "var(--doing)",
    review: "var(--review)",
    blocked: "var(--blocked)",
    done: "var(--done)"
  };

  function $(sel, root) {
    return (root || document).querySelector(sel);
  }

  function escapeHtml(value) {
    return String(value == null ? "" : value)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;")
      .replace(/'/g, "&#39;");
  }

  function unique(values) {
    var seen = {};
    var out = [];
    values.forEach(function (value) {
      if (value && !seen[value]) {
        seen[value] = true;
        out.push(value);
      }
    });
    return out;
  }

  function streamLabel(data, id) {
    var found = (data.streams || []).filter(function (s) { return s.id === id; })[0];
    return found ? found.label : id;
  }

  function loadTheme() {
    var saved = localStorage.getItem("admatrix-kanban-theme");
    if (saved === "light" || saved === "dark") return saved;
    return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  }

  function applyTheme(theme) {
    document.documentElement.setAttribute("data-theme", theme);
    localStorage.setItem("admatrix-kanban-theme", theme);
    var btn = $("#theme-btn");
    if (btn) btn.textContent = theme === "dark" ? "Light mode" : "Dark mode";
  }

  function cardMatches(card, state, data) {
    if (state.owner && card.owner !== state.owner) return false;
    if (state.stream && card.stream !== state.stream) return false;
    if (state.bottleneckOnly && !card.bottleneck) return false;
    if (state.query) {
      var hay = [
        card.id,
        card.title,
        card.owner,
        card.status,
        card.next_step,
        card.note,
        card.head,
        card.spec,
        streamLabel(data, card.stream)
      ].join(" ").toLowerCase();
      if (hay.indexOf(state.query) === -1) return false;
    }
    return true;
  }

  function chip(cls, text) {
    return '<span class="chip ' + cls + '">' + escapeHtml(text) + "</span>";
  }

  function renderCard(card, data) {
    var color = STREAM_COLORS[card.stream] || "#78716c";
    var bits = [];
    bits.push('<article class="card' + (card.bottleneck ? " bottleneck" : "") + '" style="--stream:' + color + '">');
    bits.push('<div class="card-top">');
    bits.push('<a class="card-id" href="' + escapeHtml(card.url) + '" target="_blank" rel="noopener noreferrer">' + escapeHtml(card.id) + "</a>");
    if (card.spec) bits.push(chip("spec", "SPEC " + card.spec));
    bits.push("</div>");
    bits.push('<h3 class="card-title">' + escapeHtml(card.title) + "</h3>");
    bits.push('<div class="chips">');
    bits.push(chip("owner", card.owner));
    if (card.status) bits.push(chip("status", card.status));
    if (card.mergeable === "MERGEABLE") bits.push(chip("mergeable", "MERGEABLE"));
    if (card.mergeable === "CONFLICTING") bits.push(chip("conflicting", "CONFLICTING"));
    if (card.head) bits.push(chip("head", card.head));
    if (card.bottleneck) bits.push(chip("bottleneck", "Điểm nghẽn"));
    bits.push(chip("stream", streamLabel(data, card.stream)));
    bits.push("</div>");
    if (card.next_step) {
      bits.push('<p class="next"><strong>Tiếp theo:</strong> ' + escapeHtml(card.next_step) + "</p>");
    }
    if (card.note) bits.push('<p class="note">' + escapeHtml(card.note) + "</p>");
    bits.push("</article>");
    return bits.join("");
  }

  function renderBoard(data, state) {
    var board = $("#board");
    var visible = 0;
    board.innerHTML = data.columns.map(function (col) {
      var cards = data.cards.filter(function (card) { return card.column === col.id; });
      var shown = cards.filter(function (card) { return cardMatches(card, state, data); });
      visible += shown.length;
      var over = col.wip_limit != null && cards.length > col.wip_limit;
      var wip = col.wip_limit == null ? String(shown.length) : shown.length + "/" + col.wip_limit;
      if (shown.length !== cards.length) wip += " · " + cards.length + " tổng";
      var groups = [];
      shown.forEach(function (card) {
        var key = card.group || "";
        if (!groups.length || groups[groups.length - 1].key !== key) {
          groups.push({ key: key, label: card.group_label || "", cards: [card] });
        } else {
          groups[groups.length - 1].cards.push(card);
        }
      });
      var body = groups.map(function (group) {
        var label = group.label ? '<div class="group-label">' + escapeHtml(group.label) + "</div>" : "";
        return label + group.cards.map(function (card) { return renderCard(card, data); }).join("");
      }).join("") || '<div class="empty">Không có thẻ khớp bộ lọc.</div>';
      return (
        '<section class="column' + (over ? " over-wip" : "") + '" style="--col:' + (COL_COLORS[col.id] || "var(--todo)") + '">' +
        '<header class="col-head"><span class="col-dot"></span><span class="col-title">' + escapeHtml(col.label) + "</span>" +
        '<span class="wip' + (over ? " over" : "") + '">' + escapeHtml(wip) + "</span></header>" +
        (over ? '<div class="col-note">Vượt WIP — đây là điểm nghẽn chính</div>' : "") +
        '<div class="cards">' + body + "</div></section>"
      );
    }).join("");
    $("#result-count").textContent = "Đang hiện " + visible + "/" + data.cards.length + " thẻ";
  }

  function renderChrome(data) {
    var p = data.project;
    $("#project-name").textContent = p.name;
    setStreamColors(data.streams);
    $("#meta-line").innerHTML =
      'Repo: <a href="' + escapeHtml(p.repo_url) + '" target="_blank" rel="noopener noreferrer">' + escapeHtml(p.repo) + "</a>" +
      " · Cập nhật " + escapeHtml(p.updated_at) +
      ' · <code>main</code> <a href="' + escapeHtml(p.main_url) + '" target="_blank" rel="noopener noreferrer"><code>' + escapeHtml(p.main_commit) + "</code></a>";
    $("#m-open").textContent = p.prs_open;
    $("#m-prs").textContent = p.prs_merged;
    var doingCards = data.cards.filter(function (c) { return c.column === "doing"; }).length;
    var doingCol = data.columns.filter(function (c) { return c.id === "doing"; })[0];
    $("#m-doing").textContent = doingCards + "/" + (doingCol && doingCol.wip_limit != null ? doingCol.wip_limit : "?");
    var reviewCards = data.cards.filter(function (c) { return c.column === "review"; }).length;
    var reviewCol = data.columns.filter(function (c) { return c.id === "review"; })[0];
    var reviewLimit = reviewCol && reviewCol.wip_limit != null ? reviewCol.wip_limit : "?";
    $("#m-review").textContent = reviewCards + "/" + reviewLimit;
    var reviewMetric = $("#m-review").closest(".metric");
    if (reviewCol && reviewCol.wip_limit != null && reviewCards > reviewCol.wip_limit) {
      reviewMetric.classList.add("danger");
    } else {
      reviewMetric.classList.remove("danger");
    }
    var owners = unique(data.cards.map(function (c) { return c.owner; })).sort();
    var ownerSel = $("#filter-owner");
    owners.forEach(function (owner) {
      var opt = document.createElement("option");
      opt.value = owner;
      opt.textContent = owner;
      ownerSel.appendChild(opt);
    });
    var streamSel = $("#filter-stream");
    (data.streams || []).forEach(function (stream) {
      var opt = document.createElement("option");
      opt.value = stream.id;
      opt.textContent = stream.label;
      streamSel.appendChild(opt);
    });
    var legend = $("#legend");
    legend.innerHTML = (data.streams || []).map(function (stream) {
      return '<span><i class="swatch" style="--stream:' + (STREAM_COLORS[stream.id] || "#78716c") + '"></i>' + escapeHtml(stream.label) + "</span>";
    }).join("");
  }

  function boot(data) {
    if (!data || !data.cards || !data.columns) {
      document.body.innerHTML = "<p style='padding:24px'>Không đọc được dữ liệu Kanban.</p>";
      return;
    }
    var state = { owner: "", stream: "", bottleneckOnly: false, query: "" };
    applyTheme(loadTheme());
    renderChrome(data);
    renderBoard(data, state);

    $("#filter-owner").addEventListener("change", function (ev) {
      state.owner = ev.target.value;
      renderBoard(data, state);
    });
    $("#filter-stream").addEventListener("change", function (ev) {
      state.stream = ev.target.value;
      renderBoard(data, state);
    });
    $("#filter-bottleneck").addEventListener("click", function (ev) {
      state.bottleneckOnly = !state.bottleneckOnly;
      ev.currentTarget.setAttribute("aria-pressed", state.bottleneckOnly ? "true" : "false");
      renderBoard(data, state);
    });
    $("#search").addEventListener("input", function (ev) {
      state.query = ev.target.value.trim().toLowerCase();
      renderBoard(data, state);
    });
    $("#theme-btn").addEventListener("click", function () {
      var next = document.documentElement.getAttribute("data-theme") === "dark" ? "light" : "dark";
      applyTheme(next);
    });
  }

  function start() {
    if (window.KANBAN_DATA) {
      boot(window.KANBAN_DATA);
      return;
    }
    fetch("./data.json")
      .then(function (res) { return res.ok ? res.json() : Promise.reject(new Error(res.status)); })
      .then(boot)
      .catch(function () {
        document.body.innerHTML = "<p style='padding:24px'>Không tải được <code>data.js</code> hoặc <code>data.json</code>.</p>";
      });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", start);
  } else {
    start();
  }
})();
