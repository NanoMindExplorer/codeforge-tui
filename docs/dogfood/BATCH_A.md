# Dogfood Batch A — Core coding loop (days 1–3) · Q5.4

## Automated

| Task | Pass? | Evidence |
|------|-------|----------|
| Multi-step agent edit | ✅ | integration / headless dogfood |
| BUILD staged write | ✅ | `TestStagedWriter*`, `TestDogfood_A_BUILD_*` |
| BUILD review overlay | ✅ API | `ui/review` unit (accept/reject/toggle/apply) Q5.4 |
| `@` attach path:line | ✅ API | `filepicker` tests (gitignore, line range) |
| YOLO immediate write | ✅ | staged Act mode tests |
| DESIGN blocks writes | ✅ | design mode unit |

## Field (optional HUMAN)

| Task | Pass? | Notes |
|------|-------|-------|
| `@` attach in live TUI | ⬜ | picker open on `@` key — keys_test / smoke |
| Review overlay keyboard in live TUI | ⬜ | j/k space enter esc |

**Exit:** automated core loop green; interactive polish optional for 1:1 claim.
