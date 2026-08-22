import type { SVGProps } from "react";
import spriteUrl from "../assets/icons.svg?url";

export type IconName =
  | "funnel" | "refresh" | "download" | "flag" | "hourglass" | "user-check" | "trophy"
  | "gauge" | "scale" | "pulse" | "layers" | "cloud" | "history" | "scatter" | "timer"
  | "sitemap" | "list" | "dash" | "msg" | "friends" | "user" | "cal" | "gallery" | "mega"
  | "report" | "gear" | "search" | "bell" | "flame" | "eye" | "bolt" | "tag" | "bookmark"
  | "star" | "target" | "trend" | "check" | "clock" | "gift" | "shield" | "inbox" | "plus"
  | "device" | "hook" | "doc" | "lock" | "build" | "sun" | "mouse" | "arrow-up" | "chev-right" | "mic"
  | "";

export function Icon({
  name,
  size = 14,
  style,
  ...rest
}: { name: IconName; size?: number } & SVGProps<SVGSVGElement>) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      aria-hidden="true"
      style={{ flex: "none", display: "block", ...style }}
      {...rest}
    >
      <use href={`${spriteUrl}#i-${name}`} />
    </svg>
  );
}
