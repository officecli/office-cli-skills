package plan

import (
	"strings"
	"testing"
)

func TestBuildFrameworkBlueprintMarkdown_PPTXIncludesPagePlanning(t *testing.T) {
	markdown, err := buildFrameworkBlueprintMarkdown("pptx", `{"presentationType":"项目汇报","targetAudience":"管理层","presentationPurpose":"同步季度结果","pageCount":6,"contentStyle":"结论先行","visualEffect":"简洁可信","contentGuideline":"每页只讲一个结论","slideOutline":[{"slideIndex":1,"purpose":"封面","suggestedLayout":"title","contentFormat":"paragraph","maxItems":1,"contentRequirements":"交代主题","visualSuggestion":"hero"}]}`)
	if err != nil {
		t.Fatalf("buildFrameworkBlueprintMarkdown error: %v", err)
	}
	for _, needle := range []string{"## 框架蓝图", "### 页级规划", "第 1 页", "每页只讲一个结论"} {
		if !strings.Contains(markdown, needle) {
			t.Fatalf("markdown missing %q: %s", needle, markdown)
		}
	}
}

func TestBuildFrameworkBlueprintMarkdown_DOCXIncludesSectionPlanning(t *testing.T) {
	markdown, err := buildFrameworkBlueprintMarkdown("docx", `{"documentType":"分析报告","targetAudience":"管理层","writingGoal":"解释市场变化与建议","tone":"正式专业","lengthHint":"约 3000 字","contentGuideline":"先结论后分析","sections":[{"sectionIndex":1,"heading":"摘要","purpose":"先给结论","keyPoints":["结论","建议"],"lengthHint":"300 字"}]}`)
	if err != nil {
		t.Fatalf("buildFrameworkBlueprintMarkdown error: %v", err)
	}
	for _, needle := range []string{"## 框架蓝图", "### 章节规划", "第 1 节", "先结论后分析"} {
		if !strings.Contains(markdown, needle) {
			t.Fatalf("markdown missing %q: %s", needle, markdown)
		}
	}
}

func TestBuildFrameworkBlueprintMarkdown_XLSXIncludesWorkbookPlanning(t *testing.T) {
	markdown, err := buildFrameworkBlueprintMarkdown("xlsx", `{"workbookType":"经营分析","targetAudience":"管理层","analysisGoal":"跟踪收入与预算偏差","summaryStyle":"先摘要后明细","contentGuideline":"字段口径统一","sheets":[{"sheetIndex":1,"name":"Summary","purpose":"管理摘要","columns":["月份","收入","预算偏差"],"notes":"保留核心 KPI"}]}`)
	if err != nil {
		t.Fatalf("buildFrameworkBlueprintMarkdown error: %v", err)
	}
	for _, needle := range []string{"## 框架蓝图", "### 工作簿规划", "Summary", "字段口径统一"} {
		if !strings.Contains(markdown, needle) {
			t.Fatalf("markdown missing %q: %s", needle, markdown)
		}
	}
}
