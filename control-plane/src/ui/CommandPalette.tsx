import { useEffect, useMemo, useRef, useState, type KeyboardEvent as ReactKeyboardEvent } from "react";
import { Icon, type IconName } from "./Icon";

export type CommandPaletteItem<T extends string = string> = {
  id: T;
  label: string;
  ic?: IconName;
};

export function CommandPalette<T extends string>({
  open,
  query,
  items,
  title,
  empty,
  hint,
  placeholder,
  onQuery,
  onOpen,
  onClose,
  onPick,
}: {
  open: boolean;
  query: string;
  items: CommandPaletteItem<T>[];
  title: string;
  empty: string;
  hint: string;
  placeholder: string;
  onQuery: (q: string) => void;
  onOpen: () => void;
  onClose: () => void;
  onPick: (id: T) => void;
}) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [active, setActive] = useState(0);

  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return items;
    return items.filter((it) => it.label.toLowerCase().includes(needle) || it.id.toLowerCase().includes(needle));
  }, [items, query]);

  useEffect(() => {
    setActive(0);
  }, [query, filtered.length]);

  useEffect(() => {
    if (open) inputRef.current?.focus();
  }, [open]);

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.repeat) return;
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k" && !e.shiftKey && !e.altKey) {
        e.preventDefault();
        if (open) onClose();
        else onOpen();
        return;
      }
      if (!open) return;
      if (e.key === "Escape") {
        e.preventDefault();
        onClose();
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose, onOpen]);

  useEffect(() => {
    if (!open) return;
    const node = document.querySelector<HTMLElement>("[data-palette-active='true']");
    node?.scrollIntoView({ block: "nearest" });
  }, [active, open, filtered]);

  if (!open) return null;

  function pick(id: T) {
    onPick(id);
    onClose();
  }

  function onInputKey(e: ReactKeyboardEvent<HTMLInputElement>) {
    if (e.nativeEvent.isComposing || e.keyCode === 229) return;
    if (e.key === "ArrowDown") {
      e.preventDefault();
      if (!filtered.length) return;
      setActive((i) => (i + 1) % filtered.length);
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      if (!filtered.length) return;
      setActive((i) => (i - 1 + filtered.length) % filtered.length);
    } else if (e.key === "Enter") {
      e.preventDefault();
      const hit = filtered[active] ?? filtered[0];
      if (hit) pick(hit.id);
    }
  }

  return (
    <div
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
      style={{
        position: "fixed",
        inset: 0,
        zIndex: 200,
        background: "rgba(0,0,0,.35)",
        display: "flex",
        justifyContent: "center",
        paddingTop: "12vh",
      }}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-label={title}
        onClick={(e) => e.stopPropagation()}
        style={{
          width: "min(440px, calc(100vw - 32px))",
          maxHeight: "70vh",
          background: "var(--card)",
          border: "var(--border-card)",
          borderRadius: "var(--radius-modal)",
          boxShadow: "var(--shadow-modal)",
          display: "flex",
          flexDirection: "column",
          overflow: "hidden",
        }}
      >
        <div style={{ padding: "12px 12px 8px", borderBottom: "var(--border-divider)" }}>
          <div style={{ fontSize: 12, fontWeight: 700, color: "var(--text-3)", letterSpacing: ".4px", marginBottom: 8 }}>
            {title}
          </div>
          <div
            data-ig="search"
            style={{
              display: "flex",
              alignItems: "center",
              gap: 8,
              background: "var(--surface-2)",
              border: "1px solid var(--border)",
              borderRadius: 8,
              padding: "6px 10px",
            }}
          >
            <span data-ig-part="">
              <Icon name="search" size={14} />
            </span>
            <input
              ref={inputRef}
              className="z-field"
              value={query}
              onChange={(e) => onQuery(e.target.value)}
              onKeyDown={onInputKey}
              placeholder={placeholder}
              aria-label={title}
              autoComplete="off"
              spellCheck={false}
              style={{ border: "none", background: "transparent", padding: 0, minHeight: 0, width: "100%" }}
            />
          </div>
        </div>
        <div style={{ overflowY: "auto", padding: 8, minHeight: 80 }}>
          {filtered.length === 0 ? (
            <div style={{ padding: "18px 10px", fontSize: 13, color: "var(--text-3)", textAlign: "center" }}>{empty}</div>
          ) : (
            filtered.map((it, idx) => {
              const on = idx === active;
              return (
                <button
                  key={it.id}
                  type="button"
                  data-palette-active={on ? "true" : undefined}
                  onMouseEnter={() => setActive(idx)}
                  onClick={() => pick(it.id)}
                  style={{
                    display: "flex",
                    alignItems: "center",
                    gap: 9,
                    width: "100%",
                    minHeight: 34,
                    padding: "7px 10px",
                    borderRadius: 8,
                    fontSize: 13,
                    border: "none",
                    textAlign: "left",
                    background: on ? "var(--accent-soft)" : "transparent",
                    color: on ? "var(--accent)" : "var(--text-2)",
                    fontWeight: on ? 600 : 400,
                  }}
                >
                  {it.ic ? <Icon name={it.ic} size={15} /> : null}
                  <span style={{ flex: 1 }}>{it.label}</span>
                </button>
              );
            })
          )}
        </div>
        <div
          style={{
            padding: "8px 14px",
            borderTop: "var(--border-divider)",
            fontSize: 11,
            color: "var(--text-4)",
          }}
        >
          {hint}
        </div>
      </div>
    </div>
  );
}
