package skillmgr

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultProfileMaxTokens = 1200

type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature *float64      `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Stream      bool          `json:"stream"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type,omitempty"`
		Code    any    `json:"code,omitempty"`
	} `json:"error,omitempty"`
}

type generatedProfilePayload struct {
	SummaryZh  string   `json:"summaryZh"`
	UseCasesZh []string `json:"useCasesZh"`
}

func SkillSourceText(ctx context.Context, skill Skill) (string, error) {
	if skill.RepoPath != "" && skill.RepoSubpath != "" {
		if content, ok := gitShowFile(ctx, skill.RepoPath, skill.RepoSubpath, "SKILL.md"); ok {
			return content, nil
		}
	}
	if skill.SourcePath == "" {
		return "", errors.New("skill source path is required")
	}
	data, err := os.ReadFile(filepath.Join(skill.SourcePath, "SKILL.md"))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func GenerateSkillProfile(ctx context.Context, skill Skill, llmConfig SyncLLMConfig, sourceText string) (*SkillProfile, error) {
	llmConfig = normalizeSyncLLMConfig(llmConfig)
	if llmConfig.BaseURL == "" {
		return nil, errors.New("LLM base URL is required")
	}
	if llmConfig.APIKey == "" {
		return nil, errors.New("LLM API key is required")
	}
	if llmConfig.Model == "" {
		return nil, errors.New("LLM model is required")
	}
	sourceText = strings.TrimSpace(sourceText)
	if sourceText == "" {
		return nil, errors.New("skill source text is empty")
	}

	request := chatCompletionRequest{
		Model: llmConfig.Model,
		Messages: []chatMessage{
			{
				Role: "system",
				Content: strings.Join([]string{
					"你是一个帮助用户管理 Agent Skills 的中文产品助理。",
					"你会阅读 SKILL.md，并生成准确、克制、可操作的中文能力简介和使用案例。",
					"不要夸大能力，不要编造工具或外部依赖。",
					"只输出 JSON，不要 Markdown，不要代码块。",
				}, "\n"),
			},
			{
				Role:    "user",
				Content: skillProfilePrompt(skill, sourceText),
			},
		},
		MaxTokens: llmConfig.MaxTokens,
		Stream:    false,
	}
	if request.MaxTokens == 0 {
		request.MaxTokens = defaultProfileMaxTokens
	}
	temperature := llmConfig.Temperature
	request.Temperature = &temperature

	content, err := callChatCompletion(ctx, llmConfig, request)
	if err != nil {
		return nil, err
	}
	payload, err := parseGeneratedProfile(content)
	if err != nil {
		return nil, err
	}
	profile := &SkillProfile{
		SummaryZh:   payload.SummaryZh,
		UseCasesZh:  payload.UseCasesZh,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Model:       llmConfig.Model,
		SourceHash:  skillSourceHash(sourceText),
	}
	profile = normalizeSkillProfile(profile)
	if profile == nil || profile.SummaryZh == "" || len(profile.UseCasesZh) == 0 {
		return nil, errors.New("LLM response did not include a usable profile")
	}
	return profile, nil
}

func skillProfilePrompt(skill Skill, sourceText string) string {
	sourceText = truncateRunes(sourceText, 24000)
	return fmt.Sprintf(`请根据下面的 Skill 信息生成中文说明。

输出必须是 JSON 对象，字段如下：
{
  "summaryZh": "2 到 4 句中文，说明这个 skill 的核心能力、适合场景和边界。",
  "useCasesZh": ["3 到 5 个中文使用案例，每个案例一句话，尽量具体。"]
}

Skill 名称：%s
显示名称：%s
仓库：%s
子路径：%s
已有描述：%s

SKILL.md:
%s`, skill.Name, skill.DisplayName, skill.RepoID, skill.RepoSubpath, skill.Description, sourceText)
}

func callChatCompletion(ctx context.Context, llmConfig SyncLLMConfig, request chatCompletionRequest) (string, error) {
	endpoint, err := chatCompletionEndpoint(llmConfig.BaseURL)
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+llmConfig.APIKey)
	httpRequest.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 45 * time.Second}
	response, err := client.Do(httpRequest)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return "", err
	}
	var decoded chatCompletionResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return "", fmt.Errorf("LLM response was not valid JSON: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if decoded.Error != nil && decoded.Error.Message != "" {
			return "", fmt.Errorf("LLM request failed: %s", decoded.Error.Message)
		}
		return "", fmt.Errorf("LLM request failed with status %s", response.Status)
	}
	if decoded.Error != nil && decoded.Error.Message != "" {
		return "", fmt.Errorf("LLM request failed: %s", decoded.Error.Message)
	}
	if len(decoded.Choices) == 0 || strings.TrimSpace(decoded.Choices[0].Message.Content) == "" {
		return "", errors.New("LLM response did not include message content")
	}
	return decoded.Choices[0].Message.Content, nil
}

func chatCompletionEndpoint(baseURL string) (string, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return "", errors.New("LLM base URL is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid LLM base URL: %s", baseURL)
	}
	path := strings.TrimRight(parsed.Path, "/")
	if !strings.HasSuffix(path, "/chat/completions") {
		path += "/chat/completions"
	}
	parsed.Path = path
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func truncateRunes(value string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes])
}

func parseGeneratedProfile(content string) (generatedProfilePayload, error) {
	content = strings.TrimSpace(content)
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end < start {
		return generatedProfilePayload{}, errors.New("LLM response did not contain a JSON object")
	}
	var payload generatedProfilePayload
	if err := json.Unmarshal([]byte(content[start:end+1]), &payload); err != nil {
		return generatedProfilePayload{}, fmt.Errorf("could not parse generated profile: %w", err)
	}
	payload.SummaryZh = strings.TrimSpace(payload.SummaryZh)
	payload.UseCasesZh = cleanProfileUseCases(payload.UseCasesZh)
	return payload, nil
}

func skillSourceHash(sourceText string) string {
	sum := sha256.Sum256([]byte(sourceText))
	return hex.EncodeToString(sum[:])
}
