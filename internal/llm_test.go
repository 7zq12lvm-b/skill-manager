package skillmgr

import "testing"

func TestChatCompletionEndpoint(t *testing.T) {
	tests := map[string]string{
		"https://api.deepseek.com":                    "https://api.deepseek.com/chat/completions",
		"https://api.example.com/v1/":                 "https://api.example.com/v1/chat/completions",
		"https://api.example.com/v1/chat/completions": "https://api.example.com/v1/chat/completions",
	}
	for input, expected := range tests {
		actual, err := chatCompletionEndpoint(input)
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", input, err)
		}
		if actual != expected {
			t.Fatalf("expected %q to resolve to %q, got %q", input, expected, actual)
		}
	}
}

func TestParseGeneratedProfile(t *testing.T) {
	payload, err := parseGeneratedProfile(`Here is JSON:
{
  "summaryZh": "  可用于审阅代码并指出风险。  ",
  "useCasesZh": ["检查 PR 的回归风险。", "", "总结缺失测试。"]
}
`)
	if err != nil {
		t.Fatal(err)
	}
	if payload.SummaryZh != "可用于审阅代码并指出风险。" {
		t.Fatalf("unexpected summary: %q", payload.SummaryZh)
	}
	if len(payload.UseCasesZh) != 2 || payload.UseCasesZh[0] != "检查 PR 的回归风险。" || payload.UseCasesZh[1] != "总结缺失测试。" {
		t.Fatalf("unexpected use cases: %#v", payload.UseCasesZh)
	}
}

func TestTruncateRunes(t *testing.T) {
	if actual := truncateRunes("你好世界", 2); actual != "你好" {
		t.Fatalf("expected rune-safe truncation, got %q", actual)
	}
}
