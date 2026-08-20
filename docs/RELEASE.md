# RELEASE — GOSO checklist

Phát hành GOSO core (gateway + control-plane). Không gồm ZaloCRM (SPEC 014/015).

Phiên bản hiện tại: **0.1.0** (xem `gateway/cmd/goso-gateway/main.go` const `version` và `control-plane/package.json`).

## 1. Trước khi tag

- [ ] Nhánh sạch, PR đã review, CI xanh.
- [ ] `make verify` xanh local (vet + fmt + test + gitleaks/semgrep nếu có + e2e).
- [ ] Không secret trong tree: `gitleaks detect --source . --no-git --redact --exit-code 1`.
- [ ] Semgrep local rules xanh: `semgrep --config .semgrep.yml --error --metrics off`.
- [ ] `./scripts/e2e.sh` xanh (healthz → agents → sessions → chat → webhook).
- [ ] `go run ./gateway/cmd/goso-gateway version` in JSON đúng version sắp tag.
- [ ] `go run ./gateway/cmd/goso-gateway doctor` `"ok": true`.
- [ ] Bump version **đồng bộ**:
  - `gateway/cmd/goso-gateway/main.go` (`const version`)
  - `control-plane/package.json` (`version`)
- [ ] Cập nhật `CHANGELOG.md` (mục version, ngày, Added/Changed/Fixed).
- [ ] Docs: `docs/SETUP.md`, `docs/RUNBOOK.md`, `docs/ARCHITECTURE.md` khớp hành vi.

## 2. Tag & changelog

```bash
# ví dụ v0.1.0
git add CHANGELOG.md gateway/cmd/goso-gateway/main.go control-plane/package.json
git commit -m "chore: release v0.1.0"
git tag -a v0.1.0 -m "GOSO v0.1.0"
git push origin HEAD
git push origin v0.1.0
```

Tag semver `vMAJOR.MINOR.PATCH`. GitHub Release: dán phần CHANGELOG của version đó.

## 3. Sau khi tag

- [ ] CI trên tag/main vẫn xanh.
- [ ] Smoke: `make build && ./bin/goso-gateway version`.
- [ ] Runbook: backup SQLite trước khi deploy lên môi trường có dữ liệu.
- [ ] Xoay token nếu bản release lộ env mẫu (không được).
- [ ] Thông báo nội bộ: version, breaking change (nếu có), file `docs/RUNBOOK.md`.

## 4. Rollback

- Checkout tag trước: `git checkout vX.Y.Z`.
- Restore SQLite từ `backups/` nếu schema đổi (hiện migration chỉ `CREATE IF NOT EXISTS` — rollback binary thường đủ).
- Giữ `GOSO_ADMIN_TOKEN` / DB path như môi trường đang chạy.

## 5. Không làm ở bước release này

- Pentest chính thức
- SLA 99.9%
- K8s/Helm, CI deploy tự động
- ZaloCRM / SPEC 014 / SPEC 015
