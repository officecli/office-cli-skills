# OfficeCLI 海外宣传视频 Prompt

本文档提供一份可直接用于 AI 视频生成工具的英文 prompt，目标是生成一条面向海外市场的产品宣传视频。内容基于当前仓库已确认能力编写，避免把尚未闭环的功能宣传成已上线。

适用工具：

- Runway
- Veo
- Kling
- Pika
- 其他支持长文本视频描述的生成工具

默认成片参数：

- 时长：45-60 秒
- 比例：16:9
- 语言：English
- 形式：字幕驱动，无旁白
- 风格：modern SaaS product promo, Notion/Canva-inspired

## Main Prompt

```text
Create a 45-60 second product promo video for "OfficeCLI", aimed at an overseas audience. The video should feel like a modern SaaS commercial in the style of Notion, Canva, and other clean high-conviction software ads: polished, minimal, premium, bright, fast, and product-led. No voiceover. All communication should happen through concise on-screen English captions and strong visual storytelling.

OfficeCLI is an AI-powered document generation product that turns natural language into production-ready PPTX, DOCX, and XLSX files. It is terminal-first, automation-friendly, and connected to a platform for pricing, API keys, usage tracking, billing, downloads, and online preview workflows.

The visual language should emphasize:
- clean UI mockups
- modern dashboard interfaces
- terminal commands with elegant motion
- generated presentation slides, Word-style documents, and spreadsheet views
- fast transitions between prompt input, generation progress, and polished output
- a premium product marketing feel, not a generic AI sci-fi montage

Color and art direction:
- clean light-to-neutral product marketing palette
- subtle gradients
- crisp UI surfaces
- restrained motion graphics
- premium typography
- no dark cyberpunk overload
- no cheap 3D
- no noisy glitch effects

Video structure:

Scene 1, 0-5s:
Show the pain of manual document work. Open on scattered files, repetitive copy-paste, unfinished slides, and overloaded workflows. Keep it elegant and minimal, not chaotic comedy.
On-screen caption:
"Documents still take too long to make."

Scene 2, 5-10s:
Transition into the product. Show a clean terminal and a simple prompt entry, such as:
officecli generate --prompt "Create a quarterly business review with revenue charts" --format pptx
Animate the command smoothly, with the interface feeling real, premium, and credible.
On-screen caption:
"Describe what you need in plain English."

Scene 3, 10-16s:
Show OfficeCLI transforming the request into structured output. Visualize generation progress, layout planning, content blocks, charts, and slide composition forming in sequence.
On-screen caption:
"OfficeCLI turns prompts into real deliverables."

Scene 4, 16-23s:
Reveal multiple output types in a fast, visually satisfying sequence:
- a polished presentation deck
- a clean business document
- a structured spreadsheet
Make PPTX, DOCX, and XLSX visibly distinct and premium.
On-screen caption:
"Generate PPTX, DOCX, and XLSX in one workflow."

Scene 5, 23-30s:
Focus on the presentation workflow. Show the PPTX result becoming more visually complete with embedded imagery, title slides, content slides, diagrams, and charts. Make it clear the result is presentation-ready.
On-screen caption:
"Presentation-ready output, with visuals built in."

Scene 6, 30-37s:
Show online preview and delivery. A generated file appears, then an online preview link or hosted preview page opens in a browser. Keep the experience sleek and practical.
On-screen caption:
"Generate locally. Share instantly."

Scene 7, 37-44s:
Show quality and control. Display a review flow for a PPTX deck: structure check, visual check, quality score, and refinement signals. Then transition into platform views for usage, API keys, billing, and downloads.
On-screen caption:
"Built for production, not just demos."

Scene 8, 44-52s:
Show how it fits into modern workflows. Blend terminal, automation, agent-driven execution, and product platform interfaces into one connected system. The product should feel useful for teams, operators, founders, and modern businesses.
On-screen caption:
"From single prompts to repeatable workflows."

Scene 9, 52-60s:
End on a premium hero shot with the OfficeCLI brand, clean product UI, generated assets floating in a disciplined layout, and a strong call to action.
On-screen caption:
"OfficeCLI"
"AI-powered document generation for modern teams."
"Create faster. Deliver better."

Product truths that must be reflected visually:
- natural language document generation
- support for PPTX, DOCX, and XLSX
- terminal-first workflow
- PPTX output can include built-in visuals/images
- online preview / hosted sharing feel
- review and quality-check workflow for presentations
- platform-backed pricing, API key, billing, and usage management
- suitable for workflows, automation, and agent-style execution

Design requirements:
- all UI should look realistic, premium, and internally consistent
- captions should be short, sharp, global, and native-sounding in English
- motion should be smooth and intentional, similar to premium SaaS launch videos
- use close-up product shots, interface zooms, soft parallax, clean screen replacements, and elegant transitions
- keep the pacing confident and modern
- keep people footage minimal or fully absent; the product should be the hero

Do not present the product as:
- a generic office editor
- a consumer note-taking app
- a chat-first toy
- a fictional enterprise with fake logos and fake metrics

Do not claim or imply:
- mature referral rewards systems
- Discord reward automation
- attribution or invite-loop systems already fully live
- unsupported enterprise promises

Make the final video feel credible, premium, international, software-native, and conversion-oriented.
```

## Negative Prompt

```text
Avoid generic AI video aesthetics, cyberpunk cityscapes, robots, holograms, floating brains, abstract neural networks, purple neon overload, fake coding screens, broken terminal typography, low-quality dashboard UI, inconsistent product branding, unrealistic hands typing, cheesy office stock footage, smiling call-center teams, exaggerated reaction shots, cluttered layouts, low-resolution text, warped interface elements, noisy transitions, glitch spam, cheap 3D icons, childish startup visuals, fake enterprise customer logos, fake testimonials, fake user counts, fake financial numbers, referral or reward systems presented as fully shipped, Discord growth automation claims, or any feature that looks unsupported by a real product.
```

## Subtitle Lines

可直接作为字幕底稿使用，也可按工具生成结果微调：

```text
Documents still take too long to make.
Describe what you need in plain English.
OfficeCLI turns prompts into real deliverables.
Generate PPTX, DOCX, and XLSX in one workflow.
Presentation-ready output, with visuals built in.
Generate locally. Share instantly.
Built for production, not just demos.
From single prompts to repeatable workflows.
AI-powered document generation for modern teams.
Create faster. Deliver better.
```

## Optional Short Variant

如果你要压缩成 20-30 秒短广告，可以用下面这版：

```text
Create a 20-30 second premium SaaS promo video for OfficeCLI, a product that turns natural language into production-ready PPTX, DOCX, and XLSX files. Style: modern, clean, product-led, bright, premium, similar to top-tier Notion or Canva ads. No voiceover, only concise English captions.

Show:
- manual document work feeling slow and repetitive
- a clean terminal command generating a document from a natural language prompt
- a polished PPTX deck, DOCX report, and XLSX sheet appearing in rapid sequence
- presentation output with visuals already embedded
- an online preview or hosted result page
- platform views for usage, API keys, billing, and workflow readiness

Use captions:
"Describe it."
"Generate it."
"Share it."
"Scale it."
"OfficeCLI"
"AI-powered document generation."

Keep the product as the hero. Make it feel credible, global, fast, and premium.
```

## Usage Notes

- 如果视频工具支持“参考风格 + 镜头脚本 + 负面提示”分栏输入，建议拆成 `Main Prompt + Negative Prompt + Subtitle Lines` 三段分别投喂。
- 如果工具经常把终端和 UI 生坏，可以先出“纯品牌感版本”，再单独生成几段产品界面镜头，最后剪辑拼接。
- 如果你后面要做 `9:16` 版本，建议保留字幕在安全区中央，避免终端命令和 UI 贴边。
- 如果你希望更偏开发者市场，可以把 terminal、agent bridge、automation 的镜头占比再提高。
