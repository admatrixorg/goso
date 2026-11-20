import { describe, it, expect } from "vitest";
import {
  mergeAccountDirectory,
  selectableAccounts,
  type ListUsersItem,
} from "./account-directory";

const T_MASTER = "tenant-master";
const T_AMZ = "tenant-amz";
const T_TUTU = "tenant-tutu";

function user(over: Partial<ListUsersItem> & { id: string }): ListUsersItem {
  return { email: `${over.id}@x.vn`, status: "active", memberships: [], ...over };
}

describe("mergeAccountDirectory", () => {
  it("gop nhieu tenant thanh mot danh ba, khu trung theo id", () => {
    // Cung mot nguoi tra ve tu 2 lan goi khac nhau -> phai con 1 dong.
    const a = user({
      id: "u1", email: "mq@admatrix.vn",
      memberships: [{ tenant_id: T_MASTER, role: "owner" }, { tenant_id: T_AMZ, role: "admin" }],
    });
    const dir = mergeAccountDirectory([[a], [a]]);
    expect(dir).toHaveLength(1);
    expect(dir[0]!.tenantIds.sort()).toEqual([T_AMZ, T_MASTER].sort());
  });

  it("hop nhat tenant khi moi lan goi tra ve mot phan membership", () => {
    const dir = mergeAccountDirectory([
      [user({ id: "u1", memberships: [{ tenant_id: T_MASTER, role: "owner" }] })],
      [user({ id: "u1", memberships: [{ tenant_id: T_TUTU, role: "admin" }] })],
    ]);
    expect(dir).toHaveLength(1);
    expect(dir[0]!.tenantIds.sort()).toEqual([T_MASTER, T_TUTU].sort());
  });

  it("lay ten hien thi dau tien tim duoc", () => {
    const dir = mergeAccountDirectory([
      [user({ id: "u1", memberships: [{ tenant_id: T_MASTER, role: "owner", display_name: null }] })],
      [user({ id: "u1", memberships: [{ tenant_id: T_AMZ, role: "admin", display_name: "Trung Le" }] })],
    ]);
    expect(dir[0]!.displayName).toBe("Trung Le");
  });

  it("chiu duoc batch rong / null / thieu memberships", () => {
    const dir = mergeAccountDirectory([null, undefined, [], [user({ id: "u1", memberships: null })]]);
    expect(dir).toHaveLength(1);
    expect(dir[0]!.tenantIds).toEqual([]);
  });

  it("sap xep theo email cho de tim", () => {
    const dir = mergeAccountDirectory([[
      user({ id: "u2", email: "zeta@x.vn" }),
      user({ id: "u1", email: "alpha@x.vn" }),
    ]]);
    expect(dir.map((a) => a.email)).toEqual(["alpha@x.vn", "zeta@x.vn"]);
  });
});

describe("selectableAccounts", () => {
  const dir = mergeAccountDirectory([[
    user({ id: "u1", email: "mq@admatrix.vn", memberships: [{ tenant_id: T_MASTER, role: "owner" }] }),
    user({ id: "u2", email: "trungle@amzholdings.vn", memberships: [{ tenant_id: T_AMZ, role: "admin" }] }),
    user({ id: "u3", email: "tutu@gmail.com", memberships: [{ tenant_id: T_TUTU, role: "admin" }] }),
    user({ id: "u4", email: "cu@amzholding.vn", status: "suspended", memberships: [] }),
  ]]);

  it("bo nguoi da o san trong tenant dang xem", () => {
    const got = selectableAccounts(dir, T_AMZ).map((a) => a.email);
    expect(got).not.toContain("trungle@amzholdings.vn");
  });

  it("van cho chon nguoi dang o tenant KHAC — day chinh la tinh nang can", () => {
    const got = selectableAccounts(dir, T_AMZ).map((a) => a.email);
    expect(got).toContain("mq@admatrix.vn");
    expect(got).toContain("tutu@gmail.com");
  });

  it("bo tai khoan da bi khoa", () => {
    const got = selectableAccounts(dir, T_AMZ).map((a) => a.email);
    expect(got).not.toContain("cu@amzholding.vn");
  });

  it("loc theo tu khoa tren email", () => {
    const got = selectableAccounts(dir, T_AMZ, "tutu").map((a) => a.email);
    expect(got).toEqual(["tutu@gmail.com"]);
  });

  it("tu khoa khong khop thi tra ve rong, khong no", () => {
    expect(selectableAccounts(dir, T_AMZ, "khongcoai")).toEqual([]);
  });
});
