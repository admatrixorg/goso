# Learnings — GOSO

| Feature | What worked | What tripped us | Rule to change |
|---------|-------------|-----------------|----------------|
| SPEC 008 observe | Wrap `llm.Provider` ở main → mọi channel/chat tự có trace, không sửa Telegram/Zalo | ResponseWriter wrap phải implement `Hijacker` nếu không `/ws` upgrade gãy | Middleware HTTP: luôn forward Hijack/Flush; access log chỉ `URL.Path` |

*Sau mỗi feature ghi 3 dòng retro. Bài học lặp ≥2 lần → nâng thành rule trong CLAUDE.md.*
