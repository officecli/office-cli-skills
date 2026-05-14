# officecli-skills generation demos

Each demo includes a preview image, generated artifact, prompt, metadata, and a reproducible OfficeCLI command.

| Demo | Type | Preview | Artifact | Reproduce |
| --- | --- | --- | --- | --- |
| OfficeCLI Skills customer brief | `docx` | [preview](./docx-brief/preview.png) | [officecli-skills-customer-brief.docx](./docx-brief/officecli-skills-customer-brief.docx) | `officecli new docx "OfficeCLI Skills customer brief" --prompt-file ./prompt.md --local-preview --no-publish` with [prompt](./docx-brief/prompt.md) |
| Image-rich strategy deck | `pptx` | [preview](./pptx-image-rich/preview.png) | [image-rich-strategy-deck.pptx](./pptx-image-rich/image-rich-strategy-deck.pptx) | `officecli new pptx "Image-rich strategy deck" --prompt-file ./prompt.md --local-preview --no-publish` with [prompt](./pptx-image-rich/prompt.md) |
| Text-only executive briefing | `pptx` | [preview](./pptx-text-only/preview.png) | [text-only-executive-briefing.pptx](./pptx-text-only/text-only-executive-briefing.pptx) | `officecli new pptx "Text-only executive briefing" --prompt-file ./prompt.md --local-preview --no-publish --no-images` with [prompt](./pptx-text-only/prompt.md) |
| Demo program readiness report | `report` | [preview](./report-workbook/preview.png) | [demo-program-readiness-report.html](./report-workbook/demo-program-readiness-report.html) | `officecli new report "Demo program readiness report" --file ./demo-program-source-workbook.xlsx --prompt-file ./prompt.md --no-publish` with [prompt](./report-workbook/prompt.md) |
| OfficeCLI deadline automation image | `img` | [preview](./standalone-img/preview.png) | [officecli-skills-hero-image.png](./standalone-img/officecli-skills-hero-image.png) | `officecli new img "OfficeCLI deadline automation image" --prompt-file ./prompt.md --ratio landscape --no-publish` with [prompt](./standalone-img/prompt.md) |
| Demo adoption dashboard | `xlsx` | [preview](./xlsx-dashboard/preview.png) | [demo-adoption-dashboard.xlsx](./xlsx-dashboard/demo-adoption-dashboard.xlsx) | `officecli new xlsx "Demo adoption dashboard" --prompt-file ./prompt.md --local-preview --no-publish` with [prompt](./xlsx-dashboard/prompt.md) |

## Verification

The source repository validates this directory with:

```bash
scripts/validate-skills-demos.py public/skills-demos
```

Every artifact is kept under 3MB so the public skills repository stays lightweight.
