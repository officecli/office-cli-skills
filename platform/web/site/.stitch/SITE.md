# OfficeCLI Site Plan

This document tracks the sitemap, Stitch references, delivery status, and baton direction for `platform/web/site`.

## Site intent

The OfficeCLI marketing site is responsible for:

- explaining product value
- defining the boundary between the CLI and platform
- onboarding downloads, docs, and pricing understanding
- sending login, purchasing, API key management, and order activity to `platform.officecli.io`

This site is an English-first product narrative layer for developers and purchasing stakeholders. It is not the console itself.

## Language and brand baseline

- All live site copy is English-only.
- All new Stitch screens, React implementations, content sources, and acceptance screenshots must use English as the baseline language.
- The visible brand is `OfficeCLI`.
- Lowercase slug-style references use `officecli`.

## Stitch project

- Stitch project URL: `https://stitch.withgoogle.com/projects/24832296914447158`
- Local export source: `/Users/luyang/Downloads/stitch-officecli.zip`
- Imported screen set: `home`

## Current route map

| page | route | design source | code status | note |
| --- | --- | --- | --- | --- |
| home | `/` | imported from Stitch export | coded | reference implementation for the refreshed system |
| pricing | `/pricing` | no dedicated Stitch screen yet | coded | next likely candidate for dedicated Stitch refinement |
| download | `/download` | no dedicated Stitch screen yet | coded | keeps onboarding and platform-boundary messaging |
| docs | `/docs` | no dedicated Stitch screen yet | coded | keeps docs and platform entrypoints clear |
| faq | `/faq` | no dedicated Stitch screen yet | coded | mirrors assurance and trust messaging |
| login | `/login` | no dedicated Stitch screen yet | coded | stays focused on platform redirection rationale |

## Acceptance checklist

Every new or refactored page should satisfy these checks:

- a traceable Stitch screen or explicit design source exists
- a local screenshot or export reference exists
- CTAs still respect the platform domain strategy
- desktop and mobile layouts remain usable
- at least one key-path test covers the route
- English-only copy and `OfficeCLI` naming remain intact
