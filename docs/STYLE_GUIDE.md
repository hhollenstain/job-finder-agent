# UI style guide

The browser UI is three files under `web/`: `index.html`, `styles.css`, and `app.js`. There is no CSS framework or build step. Consistency comes from one set of design tokens at the top of `styles.css` and a small set of component classes that `app.js` reuses.

You can see every component, drawn with the real stylesheet, at [`/styleguide.html`](http://127.0.0.1:8787/styleguide.html) while the app is running. The source is `web/styleguide.html`. When you add or change a component, update that page too.

## Rules

1. **Use tokens, not raw values.** Component rules use `var(--…)` for every color, font size, weight, space, radius, shadow, and transition. If nothing fits, add a token to `:root` instead of a one-off value.
2. **One control height.** Buttons, inputs, selects, the settings gear, and the star use `--control-h` (2.25rem). Chips use `--chip-h` (1.75rem).
3. **Shape follows role.**
   | Radius | Used for |
   | --- | --- |
   | `--radius-sm` (6px) | Small marks: logos, the brand mark, inline code, tab and menu items |
   | `--radius-md` (10px) | Controls: buttons, inputs, selects, the tab bar, menus, notices, score tiles |
   | `--radius-lg` (14px) | Surfaces: cards, the detail panel, job rows |
   | `--radius-pill` | Chips and badges only |
4. **Headings have one size each.** `h1` is the page title, `h2` titles a card or the detail panel, `h3` is a section heading. Element rules set the size and remove the default margins, so layout spacing comes from the container's `gap`.
5. **Containers own spacing.** `.page`, `.card`, `.stack`, and `.row` lay out their children with `gap`. Avoid margins on children except for headings inside the scrolling detail panel.
6. **Color carries meaning.** Accent is brand, success, and "on". Warn is pay, mid scores, and favorites. Danger is destructive actions, low scores, and rejected or dismissed roles. Don't use them for decoration.
7. **Focus is always visible.** Interactive elements get the global `:focus-visible` outline. Inputs show `--ring` on focus instead.

## Tokens

| Group | Tokens |
| --- | --- |
| Surfaces | `--bg`, `--surface`, `--surface-sunken`, `--surface-muted` |
| Text | `--ink`, `--ink-soft`, `--muted`, `--faint`, `--on-accent` |
| Lines | `--line`, `--line-strong`, `--line-hover`, `--track` |
| Accent | `--accent`, `--accent-hover`, `--accent-soft`, `--accent-border`, `--accent-ring` |
| Warn | `--warn`, `--warn-soft`, `--warn-border` |
| Danger | `--danger`, `--danger-soft`, `--danger-border` |
| Type | `--font-sans`, `--font-mono`, `--text-xs` … `--text-2xl`, `--weight-regular`, `--weight-medium`, `--weight-bold`, `--leading`, `--leading-tight`, `--tracking-tight`, `--tracking-caps` |
| Space | `--space-1` (0.25rem), `--space-2` (0.5rem), `--space-3` (0.75rem), `--space-4` (1rem), `--space-5` (1.5rem), `--space-6` (2rem) |
| Depth | `--shadow-sm`, `--shadow`, `--shadow-lg`, `--ring` |
| Motion | `--ease` |

### Type scale

| Token | Size | Used for |
| --- | --- | --- |
| `--text-2xl` | 1.625rem | `h1` page titles |
| `--text-xl` | 1.375rem | `h2` card and panel titles, the compensation figure |
| `--text-lg` | 1.125rem | `h3` section headings |
| `--text-md` | 1rem | Body copy, job titles in the list |
| `--text-sm` | 0.875rem | Every control, field labels, `.help`, `.meta`, tables, fit notes |
| `--text-xs` | 0.75rem | Badges, `.eyebrow`, table headers, source names |

Weights are 400, 600, and 700 only.

## Components

| Class | What it is | Notes |
| --- | --- | --- |
| `button.primary` | Main action | One per form or panel. |
| `button.ghost`, `a.ghost` | Secondary action | Links styled as buttons use `a.ghost`. |
| `button.danger` | Destructive action | Pair with a `confirm()` for anything that deletes. |
| `button.linkish` | Inline text action | For navigation inside rendered markdown. |
| `button.gear`, `button.star` | Icon buttons | Need an `aria-label`. |
| `.tabs` | Segmented navigation | The active button gets `.active`. |
| `button.stat`, `button.source-chip` | Filter chips | Toggle `.on` and `aria-pressed` together. |
| `.pill` | Status badge | Add the status name as a class (`matched`, `saved`, `rejected`, `dismissed`). |
| `.score` | Model score tile | `.high` at 70 and up, `.low` under 50. |
| `label.field` | Labelled input | First `span` is the label. An optional `span.help` follows the input. |
| `.switch-field` | On/off form setting | Built with `toggle(label, name, checked)` in `app.js`. |
| `button.switch` | Instant on/off toggle | For settings saved on click, such as enabling a source. Uses `role="switch"`. |
| `.check` | Compact checkbox | Header toolbar only (Skip cache). Forms use `.switch-field`. |
| `.card` | Page section | Grid with a built-in gap. |
| `.panel` | Scrolling detail pane | Used for the job detail. |
| `.job` | Job row | `.selected` adds the accent ring. |
| `.help`, `.muted`, `.meta`, `.mono`, `.eyebrow`, `.empty` | Text helpers | Prefer these to new text styles. |

## Adding UI

- Build elements with `h()` in `app.js` and reuse the classes above before adding a new one.
- If you need a new component, give it a single class, style it only with tokens, and add a sample to `web/styleguide.html` and a row to the table above.
- Check the layout under 720px wide. The small-screen rules are at the bottom of `styles.css`.
