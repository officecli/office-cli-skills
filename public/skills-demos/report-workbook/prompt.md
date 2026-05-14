# Demo program readiness report

## Prompt

Create a workbook-backed HTML report for demo program readiness. Ground the report in the source workbook and include KPI cards, findings, a chart, and an appendix table.

## Reproduce

```bash
officecli new report "Demo program readiness report" --file ./demo-program-source-workbook.xlsx --prompt-file ./prompt.md --no-publish
```
