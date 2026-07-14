package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type OpenAICompatible struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
	retries int
}

func NewOpenAICompatible(baseURL, apiKey, model string, client *http.Client) *OpenAICompatible {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	return &OpenAICompatible{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
		client:  client,
		retries: 2,
	}
}

func (c *OpenAICompatible) Summarize(ctx context.Context, input SummaryInput) (*SummaryResult, error) {
	if c.apiKey == "" {
		return nil, errors.New("LLM_API_KEY is not configured")
	}
	if c.model == "" {
		return nil, errors.New("LLM_MODEL is not configured")
	}
	payload := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: summarySystemPrompt},
			{Role: "user", Content: mustJSON(input)},
		},
		Temperature:    0.2,
		ResponseFormat: map[string]string{"type": "json_object"},
	}
	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		raw, err := c.doChat(ctx, payload)
		if err != nil {
			lastErr = err
			sleepBackoff(ctx, attempt)
			continue
		}
		result, err := ParseSummaryResult(raw)
		if err != nil {
			lastErr = err
			sleepBackoff(ctx, attempt)
			continue
		}
		return result, nil
	}
	return nil, fmt.Errorf("summarize with llm: %w", lastErr)
}

func (c *OpenAICompatible) ConvertToArticle(ctx context.Context, input ArticleInput) (*ArticleResult, error) {
	if c.apiKey == "" {
		return nil, errors.New("LLM_API_KEY is not configured")
	}
	if c.model == "" {
		return nil, errors.New("LLM_MODEL is not configured")
	}
	payload := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: articleSystemPrompt},
			{Role: "user", Content: mustJSON(input)},
		},
		Temperature:    0.25,
		ResponseFormat: map[string]string{"type": "json_object"},
	}
	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		raw, err := c.doChat(ctx, payload)
		if err != nil {
			lastErr = err
			sleepBackoff(ctx, attempt)
			continue
		}
		result, err := ParseArticleResult(raw)
		if err != nil {
			lastErr = err
			sleepBackoff(ctx, attempt)
			continue
		}
		return result, nil
	}
	return nil, fmt.Errorf("convert article with llm: %w", lastErr)
}

func (c *OpenAICompatible) doChat(ctx context.Context, payload chatRequest) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("llm temporary status: %d", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("llm status: %d path=/chat/completions body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var decoded chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode llm response: %w", err)
	}
	if len(decoded.Choices) == 0 || decoded.Choices[0].Message.Content == "" {
		return nil, errors.New("llm response has no content")
	}
	return []byte(decoded.Choices[0].Message.Content), nil
}

func sleepBackoff(ctx context.Context, attempt int) {
	if attempt <= 0 {
		return
	}
	timer := time.NewTimer(time.Duration(attempt*attempt) * 200 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func mustJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

type chatRequest struct {
	Model          string            `json:"model"`
	Messages       []chatMessage     `json:"messages"`
	Temperature    float64           `json:"temperature"`
	ResponseFormat map[string]string `json:"response_format,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

const summarySystemPrompt = `Ты формируешь тематическую сводку на русском языке по сообщениям Telegram.
Сначала разбей весь набор сообщений на самостоятельные темы, даже если они пришли из одной группы источников.
Каждая тема должна объединять сообщения об одном событии, вопросе или сюжете; не ограничивайся заранее заданной темой.
Не смешивай несвязанные сюжеты в одной теме. Игнорируй рекламные и сервисные сообщения.
Не выдумывай факты. Отделяй факты от мнений. Отмечай противоречия и низкую уверенность.
Для каждой темы укажи source_indexes всех сообщений, на которых она основана.
Верни только JSON: {"title":"string","overview":"string","topics":[{"title":"string","category":"string","short_summary":"string","full_summary":"string","why_important":"string","confidence":"high|medium|low","importance":1,"source_indexes":[0]}]}.`

const articleSystemPrompt = `Ты превращаешь Telegram summary или тему summary в черновик статьи на русском языке.
Требования: не выдумывай технические детали, сохраняй фактический смысл, убирай рекламу и лишние эмодзи, исправляй обрывочные формулировки, сохраняй код, явно оформляй предупреждения и выводы.
Структура зависит от type:
- educational: Что это такое, Зачем это нужно, Как это работает, Пример, Типичные ошибки, Практические рекомендации, Итоги, Источники.
- guide: Что понадобится, Подготовка, Шаги, Проверка результата, Возможные проблемы, Итоги, Источники.
- analysis: Кратко, Что произошло, Контекст, Причины, Последствия, Риски, Аргументы сторон, Вывод, Источники.
- outline: краткий структурированный план с источниками.
- telegram_post: готовый пост с фактами и источниками.
Верни только JSON: {"title":"string","type":"educational|guide|analysis|outline|telegram_post","tags":["tag"],"content_markdown":"# ..."}.
В Markdown добавь раздел "Источники" и используй только переданные source URL.`
