<!-- SPDX-License-Identifier: GPL-2.0-or-later -->

# Theming

tally's web UI is one component layer painted by many themes. Every colour,
font and texture the UI draws is read from a CSS custom property (a **token**);
a theme is nothing but a block that fills those tokens. Components in
[`internal/web/static/app.css`](../internal/web/static/app.css) never hardcode a
colour — they reference tokens only — so a new theme is authored purely by
refilling the contract, with **no component edits**.

Themes are a personal, per-browser preference: tally is local-first (see
[epic #1](https://github.com/clarkbar-sys/tally/issues/1)), so there is no
account to store a choice on. A theme is selected by setting `data-theme` on
`<html>`; the default (Amber CRT) also applies with no attribute at all.

## Two tiers of token

| Tier | Where | Overridable per theme? | What it holds |
|------|-------|------------------------|---------------|
| **System** | `:root` | No | Spacing (`--sp-1`…`--sp-6`), radius (`--r`). Rhythm, not identity. |
| **Theme** | the `THEME CONTRACT` block | Yes — a theme owns all of them | Surfaces, ink, lines, accent, status, label hues, type, and the FX budget. |

Keeping spacing and radius out of the theme layer means every theme shares the
same physical rhythm and only its *identity* changes.

## The FX budget

Amber CRT carries a deliberate "effects budget" — phosphor bloom, a blueprint
grid, film grain, scanlines, a vignette, and a slow CRT beam. Every one of
those is expressed as a token, so a calmer theme dials them toward `0`/`none`
and the CRT character disappears **without touching a single component rule**:

| Token | Off value | Controls |
|-------|-----------|----------|
| `--phosphor` | `none` | Soft halo (text-shadow) on identity/display text |
| `--grid` | `transparent` | Blueprint grid line colour + alpha |
| `--grain-opacity` | `0` | Film-grain overlay |
| `--scan-opacity` | `0` | Scanlines + vignette overlay |
| `--beam-opacity` | `0` | Slow CRT beam sweep |

A flat, modern theme sets all five to their off value and reads calm; a retro
theme turns them up. Colour still flows through the same accent/status/ink
tokens either way.

## Adding a theme

1. Open [`app.css`](../internal/web/static/app.css) and copy the whole
   `:root, [data-theme="amber-crt"]` block.
2. Change the selector to `[data-theme="your-name"]`.
3. Set `color-scheme` (`dark` or `light` — it drives native form controls and
   scrollbars).
4. Refill **every** slot. Set colours nowhere but here.
5. For a calm theme, dial the FX budget to the off values above.

Select it by setting `data-theme="your-name"` on `<html>`. (An in-app theme
switcher — persisting the choice to `localStorage` and syncing the
`<meta name="color-scheme">`/`theme-color` tags — is tracked separately.)

### Template

```css
[data-theme="your-name"]{
  color-scheme: dark;              /* or light */

  /* Surfaces — page, raised panel, deepest inset */
  --bg:            #000000;
  --bg-raised:     #000000;
  --bg-raised-2:   #000000;

  /* Ink — primary, dimmed, faint text */
  --ink:           #ffffff;
  --ink-dim:       #cccccc;
  --ink-faint:     #999999;

  /* Lines — hairline, stronger divider */
  --line:          #222222;
  --line-strong:   #333333;

  /* Accent — the interactive/identity hue */
  --accent:        #ffffff;
  --accent-strong: #ffffff;
  --accent-ink:    #000000;        /* text/icon that sits ON the accent */
  --accent-tint:   #111111;        /* faint accent wash behind surfaces */
  --accent-glow:     rgba(255,255,255,.18);
  --accent-glow-hot: rgba(255,255,255,.55);

  /* Status */
  --good:   #4fc586;
  --danger: #ff6f5e;
  --warn:   #e6a24b;

  /* Label hues — tag chips */
  --lab-red:   #ff6f5e;
  --lab-amber: #f0a838;
  --lab-green: #4fc586;
  --lab-blue:  #97a6ff;
  --lab-pink:  #e08adf;
  --lab-cyan:  #5ad6d0;

  /* Type — display + body font stacks */
  --font-display: Georgia, serif;
  --font-body:    ui-monospace, monospace;

  /* FX budget — off values shown; turn up for a retro theme */
  --phosphor:      none;
  --flash:         #ff7a3d;
  --flash-glow:    rgba(255,122,61,.55);
  --danger-glow:   rgba(255,111,94,.5);
  --grid:          transparent;
  --grain-opacity: 0;
  --scan-opacity:  0;
  --beam-opacity:  0;
}
```

## Checklist for a good theme

- **Contrast.** Body text (`--ink` on `--bg`/`--bg-raised`) should clear WCAG AA
  (4.5:1). `--ink-dim` and `--ink-faint` are for secondary text — keep them
  legible, not decorative.
- **`--accent-ink` is the pair to `--accent`.** It's the text/icon colour that
  sits *on* a filled accent surface (primary buttons, the segmented control).
  It must contrast against `--accent`, not against `--bg`.
- **`color-scheme` must match** the theme's lightness so native controls,
  caret and scrollbars render right.
- **Test `/design`.** The widget gallery renders every component; a theme is
  done when the gallery looks right, not just the app.
