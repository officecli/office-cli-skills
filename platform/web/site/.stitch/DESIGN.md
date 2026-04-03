# OfficeCLI Stitch Design System

This file captures the active design system for `platform/web/site`, derived from the exported Stitch project and translated into an implementation-ready design language.

## Creative north star

The site should feel like a **Kinetic Terminal** rather than a generic SaaS landing page:

- dark, layered surfaces
- editorial-scale typography
- precise technical labels
- restrained neon accents
- a sense that the system is already in motion

The page should read like purchasable infrastructure, not like an online editor demo.

## Language and branding

- All live marketing copy is English-only.
- All CTA labels, metadata labels, section titles, ARIA labels, and supporting copy must be English.
- The brand is written as `OfficeCLI` in visible UI.
- Use `officecli` only for slug-style identifiers such as package names, document titles, or machine-readable references.
- Do not mix Chinese and English inside active copy blocks.

## Visual rules

- Keep at least a quarter of the page in dark breathing room; avoid overfilling the canvas.
- Prefer tonal layering over strong borders for structure.
- Use gradients and glow sparingly to emphasize live, active, or premium states.
- Keep technical labels compact, uppercase, and high-tracking.
- Hero typography should feel assertive and compressed, with supporting copy calmer and more readable.

## Structural rules

The preferred story arc is:

`Hero -> Value Props -> Workflow -> CLI Showcase -> Use Cases -> Pricing -> Assurance -> Footer`

The hero should make these ideas clear within the first screen:

- this is Office document generation
- it starts from natural language
- it runs through the CLI
- platform manages the operational and purchasing layer

## Component guidance

- Header: compact infrastructure product nav with two CTA tiers max.
- Hero: oversized statement on the left, terminal-plus-artifact panel on the right.
- Value props: 2-4 cards with slight asymmetry to avoid template sameness.
- Workflow: step-based explanation of input, execution, authorization, and output tracking.
- Pricing: free entry plus production expansion via credit packs.
- Assurance: trust content that feels closer to infrastructure documentation than basic FAQ copy.

## Code mapping

Existing CSS tokens in `src/styles.css` remain the v1 implementation layer. Preserve their role semantics while aligning them to the Stitch visual system rather than renaming everything mechanically.
