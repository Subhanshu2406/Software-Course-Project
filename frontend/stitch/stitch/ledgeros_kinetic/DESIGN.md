# Design System Specification: The Kinetic Ledger

## 1. Overview & Creative North Star: "The Observational Monolith"
This design system is built to transform complex distributed ledger data into an authoritative, high-fidelity experience. Our Creative North Star is **The Observational Monolith**. Like a precision instrument or a high-end editorial journal, the interface must feel weighted, permanent, and hyper-accurate. 

We move beyond the "SaaS-standard" look by embracing **Intentional Asymmetry** and **Tonal Depth**. Instead of boxing data into rigid, repetitive grids, we use breathing room and sophisticated layering to guide the eye. The aesthetic is a fusion of technical rigor (JetBrains Mono, dense data) and premium editorial layout (Inter, generous tracking, and subtle shifts in surface luminosity).

---

## 2. Colors & Atmospheric Depth
Our palette is rooted in a deep, obsidian-like foundation. We do not use "flat" colors; we use light and shadow to imply physical existence.

### Tonal Hierarchy (Dark Mode Primary)
- **Base Foundation:** `surface` (#131315) – The bedrock of the application.
- **Sectioning:** `surface-container-low` (#1b1b1d) – Used for large sidebar or background navigation areas.
- **Elevation:** `surface-container-highest` (#353437) – Used for floating menus or active state containers.

### The "No-Line" Rule
**Explicit Instruction:** Do not use 1px solid borders for sectioning. Structural boundaries must be defined solely through background color shifts. For example, a data table container (using `surface-container-low`) should sit on the main `background` without a containing stroke. This creates a more sophisticated, "seamless" aesthetic that feels integrated rather than partitioned.

### The "Glass & Gradient" Rule
To elevate the "Vercel-inspired" look, use Glassmorphism for transient elements (modals, dropdowns, tooltips). Use `surface-variant` with a `backdrop-filter: blur(12px)` and a 40% opacity.
- **Signature Accents:** Main CTAs should utilize a subtle linear gradient: `primary` (#c0c1ff) to `primary-container` (#8083ff) at a 135-degree angle. This adds a "soul" to the interface that flat hex codes cannot replicate.

---

## 3. Typography: Editorial Rigor
Typography is our primary tool for hierarchy. We leverage the juxtaposition of a humanistic sans-serif with a high-performance monospace.

- **Display & Headlines:** Use `Inter` at a `500` (Medium) weight. To achieve the editorial look, utilize `headline-sm` (1.5rem) with a slightly tighter letter-spacing (-0.02em).
- **The Data Layer:** All ledger hashes, transaction IDs, and numerical values must use `JetBrains Mono`. This signals technical accuracy.
- **The Label System:** `label-sm` (0.6875rem) should be used in `uppercase` with a `tracking-widest` (0.1em) setting, colored in `on-surface-variant`. This creates a sophisticated, "instrument-panel" feel.

---

## 4. Elevation & Depth: Tonal Layering
We reject the use of heavy drop shadows. Hierarchy is achieved through the **Layering Principle**.

### Tonal Stacking
Depth is created by "stacking" surface tiers.
1. **Level 0 (Floor):** `surface-dim` (#131315)
2. **Level 1 (Sections):** `surface-container-low` (#1b1b1d)
3. **Level 2 (Cards/Modules):** `surface-container` (#201f21)
4. **Level 3 (Popovers):** `surface-container-highest` (#353437)

### Ambient Shadows & Ghost Borders
- **Shadows:** Only used for floating elements. Use a large 32px blur with 4% opacity, tinted with `primary` to simulate light refracting through a lens.
- **The Ghost Border:** If containment is required for accessibility, use a "Ghost Border": `outline-variant` (#464554) at 15% opacity. **Never use 100% opaque borders.**

---

## 5. Components: Precision Primitives

### Buttons & Chips
- **Primary Action:** Gradient fill (Primary to Primary-Container), `rounded-md` (6px). No border.
- **Secondary Action:** Ghost style. No background, `Ghost Border` (15% opacity), `on-surface` text.
- **Status Chips:** Use `rounded-full` with a 10% opacity fill of the status color (e.g., `success` at 10% alpha) and a 100% opaque `status-dot`.

### Input Fields & Data Grids
- **Input Fields:** `surface-container-lowest` background. On focus, transition the "Ghost Border" to 40% `primary` opacity. 
- **Data Tables:** **Forbid divider lines.** Separate rows using 12px of vertical white space and a subtle `surface-container-low` hover state that spans the full width of the viewport.

### Special Dashboard Components
- **The "Pulse" Indicator:** For live ledger syncing, use a `primary` status dot with a CSS-animated ripple effect (20% opacity expansion).
- **The Hash-Shortener:** In lists, display JetBrains Mono hashes with the middle truncated, using `on-surface-variant` for the ellipsis to maintain scannability.

---

## 6. Do’s and Don’ts

### Do
- **Do** prioritize "Breathing Room." If a layout feels cramped, increase the padding by 1.5x.
- **Do** use `JetBrains Mono` for all numeric data—it implies the system is "live" and "technical."
- **Do** use asymmetric layouts. A 60/40 split is often more sophisticated than a 50/50 split.

### Don't
- **Don't** use 100% black (#000) or pure white (#FFF). Use our tonal tokens to maintain "atmosphere."
- **Don't** use traditional "Drop Shadows" with high opacity. They break the Monolith aesthetic.
- **Don't** use icons without labels for primary navigation. In a technical dashboard, clarity is the highest form of luxury.
- **Don't** use dividers between list items. Trust the spacing and the subtle tonal shifts of the containers.