# Design System

This document defines the visual design language for routatic-proxy's macOS GUI.

## Target Users

Software developers who manage proxy configurations and need quick diagnostic information.

## Design Principles

1. **Information density over decoration** — Every pixel should earn its place
2. **Diagnostic at a glance** — Status should be assessable in 3 seconds
3. **Power-user friendly** — Keyboard shortcuts, search, quick actions
4. **Native feel** — Follow Apple Human Interface Guidelines

## Color System

Uses CSS variables for light/dark mode support:

```css
/* Light Mode */
--bg:           #f5f5f7;  /* Background */
--surface:      #ffffff;  /* Card backgrounds */
--surface2:     #f0f0f2;  /* Secondary surfaces */
--border:       #d8d8dc;  /* Borders */
--text:         #1d1d1f;  /* Primary text */
--text-muted:   #6e6e73;  /* Secondary text */
--accent:       #0071e3;  /* Primary action color */
--success:      #28a745;  /* Success states */
--error:        #dc3545;  /* Error states */
--warning:      #fd7e14;  /* Warning states */
```

## Spacing Scale

4px base unit:

| Token | Value | Usage |
|-------|-------|-------|
| `--space-1` | 4px | Tight gaps |
| `--space-2` | 8px | Standard gaps |
| `--space-3` | 12px | Section padding |
| `--space-4` | 16px | Card padding |
| `--space-5` | 24px | Major sections |
| `--space-6` | 32px | Page margins |

## Typography Scale

System font stack (`-apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif`):

| Token | Size | Usage |
|-------|------|-------|
| `--text-xs` | 11px | Labels, hints |
| `--text-sm` | 12px | Secondary text |
| `--text-base` | 13px | Body text |
| `--text-md` | 14px | Section headers |
| `--text-lg` | 16px | Card titles |
| `--text-xl` | 26px | Metric values |

## Border Radius Hierarchy

| Token | Value | Usage |
|-------|-------|-------|
| `--radius-sm` | 4px | Buttons, inputs |
| `--radius-md` | 6px | Fieldsets, smaller cards |
| `--radius-lg` | 10px | Cards, modals |

## Accessibility

- **Touch targets**: 44px minimum (toggle switches, buttons)
- **Focus indicators**: Custom ring style with `--shadow-focus`
- **Color contrast**: WCAG AA compliant (4.5:1 for body text)
- **ARIA labels**: On all interactive elements
- **Keyboard navigation**: Full support with visible focus

## Keyboard Shortcuts

| Shortcut | Action |
|----------|--------|
| ⌘K | Open command palette |
| ⌘R | Refresh all data |
| ⌘F | Focus history search (when on History tab) |
| ⌘1 | Go to Overview tab |
| ⌘2 | Go to History tab |
| ⌘3 | Go to Settings tab |
| Escape | Close modal/command palette |

## Components

### Status Badge
- Animated dot (green for running, red for stopped)
- Text status: Running / Stopped / Connected
- Role: `status` with `aria-live="polite"`

### Metric Cards
- 4-column grid on Overview
- Value in `--text-xl` (26px bold)
- Label in `--text-xs` (11px muted)
- No decorative shadows (functional only)

### Toggle Switches
- 44px touch target (accessibility compliant)
- iOS-style slider animation
- Clear on/off visual states

### History Table
- Sortable columns (click header to sort)
- Search bar with ⌘F shortcut
- Click row for detail modal
- Pagination handled by scroll

### Settings Accordion
- Collapsed by default (progressive disclosure)
- One section expanded at a time
- Smooth expand/collapse animation
- Chevron icon indicates state

## NOT in Scope

- Responsive breakpoints (fixed 375px width for native webview)
- Mobile/tablet layouts
- Custom typography (uses system fonts)
- Animation-heavy interactions
