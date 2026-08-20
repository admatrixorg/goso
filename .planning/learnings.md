# Learnings — GOSO

| Feature | What worked | What tripped us | Rule to change |
|---------|-------------|-----------------|----------------|
| SPEC 012 Deploy | Multi-stage Go 1.25 (CGO=0) + Node 22 `server.mjs` proxy; compose overlay merge (restart/backup) không đụng ports | Host `127.0.0.1:8080` có process bind chặt hơn Docker `*:8080` (Open WebUI) — healthz vẫn xanh trong container + qua :3000 | Ghi troubleshooting bind 127.0.0.1 vs `*:port` trong DEPLOY.md; overlay không redeclare `ports` |

*Sau mỗi feature ghi 3 dòng retro. Bài học lặp ≥2 lần → nâng thành rule trong CLAUDE.md.*
