# OfficeCLI generation demos

This gallery gives GitHub visitors and search engines concrete proof that OfficeCLI is an AI document
generation CLI, not only an install script. Every demo includes a preview image, generated artifact,
prompt, metadata, and a reproducible OfficeCLI command.

## Reproducible demo matrix

| Demo | Output type | Search keywords | Preview | Artifact | Reproduce |
| --- | --- | --- | --- | --- | --- |
| Image-rich strategy deck | `pptx` | AI PPTX generator, PowerPoint automation, image-rich deck | ![AI PPTX generator preview created by OfficeCLI](./pptx-image-rich/preview.png) | [image-rich-strategy-deck.pptx](./pptx-image-rich/image-rich-strategy-deck.pptx) | `officecli new pptx "Image-rich strategy deck" --prompt-file ./prompt.md --local-preview --no-publish` with [prompt](./pptx-image-rich/prompt.md) |
| Text-only executive briefing | `pptx` | text-only AI PPTX generator, executive briefing | ![Text-only AI PPTX generator preview created by OfficeCLI](./pptx-text-only/preview.png) | [text-only-executive-briefing.pptx](./pptx-text-only/text-only-executive-briefing.pptx) | `officecli new pptx "Text-only executive briefing" --prompt-file ./prompt.md --local-preview --no-publish --no-images` with [prompt](./pptx-text-only/prompt.md) |
| OfficeCLI customer brief | `docx` | DOCX generator CLI, AI Word document generator | ![DOCX generator CLI preview created by OfficeCLI](./docx-brief/preview.png) | [officecli-customer-brief.docx](./docx-brief/officecli-customer-brief.docx) | `officecli new docx "OfficeCLI customer brief" --prompt-file ./prompt.md --local-preview --no-publish` with [prompt](./docx-brief/prompt.md) |
| Demo adoption dashboard | `xlsx` | XLSX automation, AI spreadsheet generator, dashboard workbook | ![XLSX automation preview created by OfficeCLI](./xlsx-dashboard/preview.png) | [demo-adoption-dashboard.xlsx](./xlsx-dashboard/demo-adoption-dashboard.xlsx) | `officecli new xlsx "Demo adoption dashboard" --prompt-file ./prompt.md --local-preview --no-publish` with [prompt](./xlsx-dashboard/prompt.md) |
| Demo program readiness report | `report` | report generation, workbook-backed report, XLSX to report | ![Report generation preview created by OfficeCLI](./report-workbook/preview.png) | [demo-program-readiness-report.html](./report-workbook/demo-program-readiness-report.html) | `officecli new report "Demo program readiness report" --file ./demo-program-source-workbook.xlsx --prompt-file ./prompt.md --no-publish` with [prompt](./report-workbook/prompt.md) |
| OfficeCLI deadline automation image | `img` | image generation CLI, AI image generator, standalone image | ![Image generation CLI preview created by OfficeCLI](./standalone-img/preview.png) | [officecli-hero-image.png](./standalone-img/officecli-hero-image.png) | `officecli new img "OfficeCLI deadline automation image" --prompt-file ./prompt.md --ratio landscape --no-publish` with [prompt](./standalone-img/prompt.md) |

## How to read a demo

- `preview.png` is the GitHub-friendly visual proof shown in the README gallery.
- The artifact is the generated `PPTX`, `DOCX`, `XLSX`, report, or `PNG` output.
- `prompt.md` is the reusable input for the command.
- `metadata.json` records the command, output type, verification timestamp, and checksums.

## Verification

The source repository validates this directory with:

```bash
scripts/validate-skills-demos.py public/skills-demos
```

Every artifact is kept under 3MB so the public repository stays lightweight.
