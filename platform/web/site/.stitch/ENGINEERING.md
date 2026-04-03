# OfficeCLI Stitch Engineering Guide

This document defines how Stitch designs are translated into production code for `platform/web/site`.

## Core principles

- Treat Stitch as the design source of truth and React as the production source of truth.
- Keep design assets, screenshots, and baton prompts under `.stitch` for traceability.
- Build pages as reusable React sections rather than importing exported Stitch HTML directly.
- Validate every page with route-level tests, visual inspection, and platform CTA checks.

## Language and naming rules

- `platform/web/site` is an English-only marketing site.
- All user-visible copy, ARIA labels, test baselines, and Stitch baton prompts must be written in English.
- The user-facing brand name is always `OfficeCLI`.
- The technical slug, document title, package-style identifiers, and lowercase references use `officecli`.
- Do not reintroduce `officecli`, `officecli`, or mixed Chinese and English UI copy.

## Required workflow

1. Export the approved Stitch screen into `.stitch/designs` as HTML and PNG reference assets.
2. Update `.stitch/metadata.json` and `.stitch/SITE.md` with the active route, status, and source reference.
3. Extract design tokens, content structure, and reusable sections before writing page code.
4. Implement the page in `src/pages`, supported by shared layout and marketing components.
5. Keep content in `src/content/*` or `src/siteData.ts` rather than burying copy inside JSX.
6. Run tests, build, and English-copy regression searches before considering the page complete.

## Implementation boundaries

### Stitch is allowed to provide

- Visual direction and atmosphere
- Section sequencing and information hierarchy
- Initial marketing copy concepts
- Screenshot and layout reference baselines

### Production code must provide

- Reusable React structure
- Routing and CTA correctness
- Data fetching and API integration
- Accessibility, maintainability, and long-term styling consistency
- Test coverage and release-ready artifacts

### Prohibited shortcuts

- Shipping exported Stitch HTML as the final maintainable page implementation
- Preserving deeply nested auto-generated class structures in `src`
- Copy-pasting entire pages when reusable sections should be extracted
- Letting copy drift away from the English-only and `OfficeCLI` naming rules

## Definition of done

A Stitch-backed page is complete only when all of the following are true:

- The source screen is traceable through `.stitch/metadata.json` and `.stitch/designs/*`.
- The route is implemented in React and fits the shared site shell.
- Desktop and mobile layouts are manually checked.
- All CTAs point to the correct route or `platform.officecli.io` surface.
- Route tests or key-content tests pass.
- `npm run build` passes.
- English-copy regression searches show no active Chinese UI copy and no `officecli` / `officecli` branding regressions.
