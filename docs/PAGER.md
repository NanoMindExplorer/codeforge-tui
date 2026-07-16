# pager.toml (Grok-compatible)

Fine-grained TUI chrome: scrollback layout, scrollbar, sticky headers, block styling, animation, and scroll input. Matches Grok Build’s `~/.grok/pager.toml` surface.

## Locations (last wins)

| Path | Scope |
|------|--------|
| Built-in defaults | Always |
| `~/.grok/pager.toml` | User (Grok) |
| `~/.codeforge/pager.toml` or `.yaml` | User |
| `<project>/.grok/pager.toml` | Project |
| `<project>/.codeforge/pager.toml` or `.yaml` | Project |
| `CODEFORGE_PAGER` / `GROK_PAGER` | Explicit path |

Slash: **`/pager`** status · **`/pager reload`**

## Example (`~/.codeforge/pager.toml`)

```toml
[scrollback.layout]
outer_vpad = 1
outer_hpad_left = 2
outer_hpad_right = 2
block_pad_left = 2
block_pad_right = 2

[scrollback.scrollbar]
enabled = true
gap_left = 0
gap_right = 0
# scrollbar_bg = "none"
# scrollbar_fg = "none"

[scrollback.scroll]
margin = 0
min_page_fraction = 0
follow_indicator = "center"   # center | none
follow_auto_select = true
follow_by_overscroll = true
anchor_on_fold = true

[scrollback.display]
sticky_headers = true
tab_width = 4
expandable_indicator = true
expandable_indicator_char = "›"
collapsed_accent_char = "❙"
dim_accent = 0.5
line_under_last_entry = false
selection_buttons = false

[animation]
fps = 30
wave_rows = 32

[scrollback.blocks.edit]
indent = true
vpad = false
expanded_by_default = true
hunk_separator = "…"
dual_line_numbers = false
line_summary = false

[scrollback.blocks.thinking]
accent_enabled = true
animate = true
truncate_lines = 3
bg_blend = 70
header = true
header_bright = false

[scrollback.blocks.tool]
muted_collapsed = true
dim_details = true
bullet = "diamond"          # none|dot|small-circle|circle|small-triangle|triangle|diamond

[scrollback.blocks.execute]
first_lines = 2
last_lines = 3
accent_enabled = true
header_style = "label"      # shell | label
muted_command_collapsed = true

[scrollback.blocks.prompt]
vpad = true
bg = "light"                # none | light | dark
show_prefix = true
min_lines = 2

[prompt]
collapse_unfocused = true
mouse_hover = true
show_prefix = true

[todo]
badge_format = "default"    # default | colon | comma

[terminal]
alt_screen = "auto"         # auto | always | never
# minimal = false

[ui]
max_thoughts_width = 120
show_thinking_blocks = true
group_tool_verbs = true
screen_mode = "fullscreen"  # minimal | fullscreen
scroll_speed = 50           # 1–100 (50 = 1.0×)
scroll_mode = "auto"        # auto | wheel | trackpad
# scroll_lines = 3
invert_scroll = false
simple_mode = true
# vim_mode = false
# compact_mode = false
# default_selected_permission = "always_allow_all_sessions"
# remember_tool_approvals = false

# disable_plugins = false
```

YAML is also supported (same keys, nested under `layout` / `scrollbar` / … without the `scrollback.` prefix).

## What is wired today

| Section | Effect in CodeForge |
|---------|---------------------|
| `scrollback.layout` | Viewport / block padding (`theme.CurrentLayout`) |
| `scrollback.scrollbar` | Show/hide + color overrides |
| `scrollback.display` | Sticky headers, fold chars, dim |
| `scrollback.blocks.tool` | Bullet character on tool rows |
| `scrollback.blocks.thinking` | Header, truncate lines, show/hide |
| `animation.fps` | Spinner / border animation tick rate |
| `[ui]` | Thinking width, invert scroll, scroll speed/lines, screen_mode, compact/vim |
| `[todo]` | Badge format for footer todos |
| `[terminal]` | minimal / alt_screen preference |

`config.yaml` `[ui]` keys are merged on top of pager files (same names as Grok `[ui]`).

## Related

- Grok docs: [Theming](https://x.ai) user-guide `06-theming.md`  
- [REASONING.md](./REASONING.md) · [TERMINAL_MATRIX.md](./TERMINAL_MATRIX.md)
