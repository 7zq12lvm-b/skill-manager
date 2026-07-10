# Responsive Workbench Layout

## Status

Approved for implementation on 2026-07-10.

## Problem

The application currently opens at 1024 by 768 and always renders Repositories, Skills, and Skill Detail side by side. At narrow widths the repository and detail columns consume most of the window, leaving the Skills table too narrow for its filters, tags, source, and status controls. The existing 860px media query only hides top-bar decoration and does not change the workbench structure.

## Goals

- Keep the current resizable three-panel workbench on wide windows.
- Make 1024px windows comfortable without reducing the Skills table to a narrow strip.
- Keep the application usable down to a 720px minimum window width.
- Preserve every repository, skill, bulk action, detail action, and filter workflow at each breakpoint.
- Avoid maintaining duplicate repository panel implementations.

## Breakpoints

The workbench uses three explicit layouts based on `window.innerWidth`:

- `desktop` at 1200px and above;
- `split` from 850px through 1199px;
- `compact` below 850px.

The Wails default window becomes 1280 by 820. The minimum window becomes 720 by 600 so the compact layout retains stable controls without forcing an oversized window on a small display.

## Desktop Layout

Desktop retains the current Repositories, Skills, and Skill Detail columns and both resize handles. Persisted source and detail widths continue to apply only in this layout.

Moving from a smaller breakpoint back to desktop closes the repository drawer and restores all three columns without changing the selected source, selected skill, filters, table selection, or saved panel widths.

## Split Layout

At 850px through 1199px, Repositories is removed from the grid. Skills and Skill Detail share the full workbench width, separated by the existing detail resize handle. The detail width remains constrained to a useful range while Skills receives the remaining space.

The Skills header gains a Repositories icon button with a functional tooltip. It opens the repository panel as a left-side overlay drawer, so browsing repositories does not resize or cover the Skills table permanently.

The drawer:

- reuses the same repository panel component as desktop;
- has a bounded width of 340px or the available viewport width minus a safe margin;
- closes through its close button, backdrop click, Escape, source selection, or returning to desktop;
- keeps all existing repository actions and tooltips unchanged;
- traps no application state and does not reset scrolling or filters unnecessarily.

## Compact Layout

Below 850px, the workbench displays one primary view at a time. A compact tab bar offers Skills and Detail, plus the Repositories drawer button.

- Skills is the default compact view.
- Selecting a skill row switches to Detail automatically.
- Selecting Skills in the tab bar returns to the table without losing table scroll, filters, or selection.
- Detail actions remain in the existing Skill Detail header.
- Resize handles are not rendered in compact layout.
- The hidden view remains mounted only when needed to preserve useful state without allowing hidden content to affect layout or accessibility.

When the viewport grows into split layout, both Skills and Detail become visible again. The last compact tab is remembered for the next time the window is narrowed.

## Shared Components And State

The inline repository panel is extracted into a `RepositoryPanel` component that receives repository data and existing callbacks. Desktop and drawer placements use this one implementation.

`App` owns:

- the derived workbench layout mode;
- repository drawer visibility;
- compact active view;
- the existing selected source, selected skill, filters, table selection, and panel widths.

A small viewport hook listens to `resize`, updates only when the breakpoint mode changes, and removes its listener on unmount. Layout changes do not write artificial panel widths to local storage.

## Top Bar And Overflow

At split and compact widths, the decorative sync route and caption are hidden earlier than today. Summary badges and command buttons wrap without overlapping the workbench. Panel headers keep stable heights and icon buttons rather than compressing text labels.

The Skills table continues to use its existing horizontal and vertical overflow container. Column minimums remain intact because responsive layout gives the table a meaningful width instead of squeezing all three panels.

## Accessibility

- Drawer open and close controls have descriptive labels and tooltips.
- The drawer uses dialog semantics and exposes its title.
- Escape closes the drawer.
- Compact view controls use tab semantics with selected state.
- Hidden workbench views are removed from keyboard and accessibility navigation.
- Existing hover descriptions remain unchanged.

## Verification

Automated verification includes Go tests, Go vet, TypeScript checking, frontend production build, and Wails build.

Desktop screenshots and accessibility checks cover:

- 1280px: three visible panels and resize handles;
- 1024px: Skills and Detail split view, repository drawer closed and open;
- 800px: single Skills view, single Detail view after row selection, and repository drawer;
- no overlapping headers, filters, table controls, detail actions, drawer content, or top-bar content;
- state preservation while crossing breakpoints in both directions.
