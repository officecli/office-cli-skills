# OfficeCLI Marketing Video Prompt

Use the following prompt as a baseline when creating an English-language marketing video for OfficeCLI.

## Target Outcome

Produce a short product video that communicates:

- OfficeCLI turns natural-language prompts into Office documents
- OfficeCLI can also generate standalone local images through the platform image route
- PPT generation can include automatically generated visuals
- the CLI supports local file output and optional online preview publishing
- the platform supports free quota, paid document-generation quota, and operational visibility

## Prompt Template

```text
Create a product marketing video for OfficeCLI.

Audience:
- developers
- operators
- product teams

Core message:
- OfficeCLI is a command-line tool that generates PPTX, DOCX, XLSX, workbook-backed Report, and standalone image outputs from natural-language prompts.
- PPTX generation can automatically create and embed images when appropriate.
- Standalone image generation is server-provider controlled and requires platform access.
- Users can keep output local or publish to an online preview endpoint when configured.
- The platform provides quota enforcement, billing visibility, and admin controls.

Visual direction:
- clean product-demo pacing
- modern terminal sequences
- clear UI shots of the platform app and admin pages
- no exaggerated claims about unsupported features

Mandatory constraints:
- all on-screen copy must be English
- avoid mentioning internal code names or unpublished roadmap items
- do not imply that Discord rewards or attribution analytics are fully launched unless verified
```

## Review Checklist

- All text is English-only
- Product claims match the current implementation
- Screens and command output match the current CLI and platform UI
- No skill-layer wording is used to describe behavior that actually belongs to the binary
