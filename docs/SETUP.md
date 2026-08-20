# SETUP — GOSO (5 bước)

## 1. Yêu cầu

- Go 1.23+ (`go version`)
- Git, make

## 2. Clone & cài

```bash
git clone <repo> && cd goso
go mod download
```

## 3. Chạy gateway skeleton

```bash
go run ./gateway/cmd/goso-gateway --help
go run ./gateway/cmd/goso-gateway version
go run ./gateway/cmd/goso-gateway doctor
# {"name":"goso-gateway","version":"0.1.0"} / checks ok
```

## 4. Verify harness

```bash
make verify   # vet + fmt + test
make build    # bin/goso-gateway
```

## 5. Pre-commit (tùy chọn)

```bash
ln -sf ../../scripts/pre-commit.sh .git/hooks/pre-commit  # nếu goso là repo root
# hoặc chạy thủ công: ./scripts/pre-commit.sh
```

## Biến môi trường

| Biến | Mặc định | Mô tả |
|------|----------|-------|
| GOSO_PORT | 8080 | Cổng gateway |
| GOSO_LOG_LEVEL | info | Mức log |
| GOSO_ENV | development | Môi trường |
