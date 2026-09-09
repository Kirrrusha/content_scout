package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

type OpenAICompatible struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
	retries int
	logger  *slog.Logger
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
		logger:  slog.Default(),
	}
}

// SetLogger replaces the logger used to report per-batch problems. A batch that
// fails is survivable, so it never surfaces as an error, and without a log line
// a thin digest looks the same as a complete one.
func (c *OpenAICompatible) SetLogger(logger *slog.Logger) {
	if logger != nil {
		c.logger = logger
	}
}

// Summarize splits the collection into batches before calling the model. A whole
// collection routinely carries tens of kilobytes of message text, most of it in
// Telegram captions, and asking for one summary over all of it takes longer than
// any reasonable request timeout. Batches are summarized concurrently and their
// topics merged, so each individual request stays small and predictable.
func (c *OpenAICompatible) Summarize(ctx context.Context, input SummaryInput) (*SummaryResult, error) {
	if c.apiKey == "" {
		return nil, errors.New("LLM_API_KEY is not configured")
	}
	if c.model == "" {
		return nil, errors.New("LLM_MODEL is not configured")
	}
	batches := splitSummaryMessages(input.Messages, maxBatchContentBytes)
	if len(batches) <= 1 {
		return c.summarizeBatch(ctx, input)
	}
	partials, err := c.summarizeBatches(ctx, input, batches)
	if err != nil {
		return nil, err
	}
	return c.mergeSummaries(ctx, input, partials)
}

func (c *OpenAICompatible) summarizeBatch(ctx context.Context, input SummaryInput) (*SummaryResult, error) {
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
	defer func() { _ = resp.Body.Close() }()
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

const (
	// maxBatchContentBytes bounds the message payload of one summarize request.
	// A 20 KB batch still outran a 3 minute timeout on a reasoning model, burning
	// every retry, so keep batches small enough that one attempt comfortably fits.
	maxBatchContentBytes = 6000
	// maxBatchConcurrency keeps a large collection from fanning out into an
	// unbounded burst of requests against the provider.
	maxBatchConcurrency = 3
)

// summaryBatch is a slice of the collection together with the mapping back to
// the caller's indexes. Messages are renumbered from zero inside a batch, since
// a model asked to reference source_indexes tends to number what it was given
// rather than echo the indexes it was handed.
type summaryBatch struct {
	messages      []SummaryMessageInput
	globalIndexes []int
}

func splitSummaryMessages(messages []SummaryMessageInput, maxBytes int) []summaryBatch {
	if len(messages) == 0 {
		return nil
	}
	if maxBytes <= 0 {
		maxBytes = maxBatchContentBytes
	}
	var batches []summaryBatch
	current := summaryBatch{}
	size := 0
	for i, message := range messages {
		messageSize := len(message.Text) + len(message.ChatTitle)
		// Keep at least one message per batch, however long it is on its own.
		if len(current.messages) > 0 && size+messageSize > maxBytes {
			batches = append(batches, current)
			current = summaryBatch{}
			size = 0
		}
		local := message
		local.Index = len(current.messages)
		current.messages = append(current.messages, local)
		current.globalIndexes = append(current.globalIndexes, i)
		size += messageSize
	}
	if len(current.messages) > 0 {
		batches = append(batches, current)
	}
	return batches
}

func (c *OpenAICompatible) summarizeBatches(ctx context.Context, input SummaryInput, batches []summaryBatch) ([]*SummaryResult, error) {
	results := make([]*SummaryResult, len(batches))
	errs := make([]error, len(batches))
	semaphore := make(chan struct{}, maxBatchConcurrency)
	var wg sync.WaitGroup
	for i, batch := range batches {
		wg.Add(1)
		go func(i int, batch summaryBatch) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			batchInput := SummaryInput{Language: input.Language, Format: input.Format, Messages: batch.messages}
			result, err := c.summarizeBatch(ctx, batchInput)
			if err != nil {
				errs[i] = err
				c.logger.Warn("summary batch failed",
					"batch", i+1, "batches", len(batches), "messages", len(batch.messages), "error", err)
				return
			}
			remapSourceIndexes(result, batch.globalIndexes)
			results[i] = result
		}(i, batch)
	}
	wg.Wait()
	// One failed batch still leaves a usable digest, so only give up when every
	// batch failed.
	var firstErr error
	succeeded := 0
	for i, err := range errs {
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if results[i] != nil {
			succeeded++
		}
	}
	if succeeded == 0 {
		if firstErr == nil {
			firstErr = errors.New("no batch produced a summary")
		}
		return nil, firstErr
	}
	c.logger.Info("summary batches finished", "batches", len(batches), "succeeded", succeeded)
	return results, nil
}

func remapSourceIndexes(result *SummaryResult, globalIndexes []int) {
	for i := range result.Topics {
		mapped := make([]int, 0, len(result.Topics[i].SourceIndexes))
		for _, local := range result.Topics[i].SourceIndexes {
			if local < 0 || local >= len(globalIndexes) {
				continue
			}
			mapped = append(mapped, globalIndexes[local])
		}
		result.Topics[i].SourceIndexes = mapped
	}
}

// mergeSummaries collects the batch topics and asks the model for one title and
// overview over them. The reduce request only carries topic headlines, so it is
// small; if it fails the digest is still returned with a locally built overview.
func (c *OpenAICompatible) mergeSummaries(ctx context.Context, input SummaryInput, partials []*SummaryResult) (*SummaryResult, error) {
	merged := &SummaryResult{}
	overviews := make([]string, 0, len(partials))
	for _, partial := range partials {
		if partial == nil {
			continue
		}
		merged.Topics = append(merged.Topics, partial.Topics...)
		if strings.TrimSpace(partial.Overview) != "" {
			overviews = append(overviews, strings.TrimSpace(partial.Overview))
		}
		if merged.Title == "" {
			merged.Title = partial.Title
		}
	}
	merged.Overview = strings.Join(overviews, " ")
	if headline, err := c.summarizeHeadline(ctx, input, merged.Topics); err == nil {
		if strings.TrimSpace(headline.Title) != "" {
			merged.Title = headline.Title
		}
		if strings.TrimSpace(headline.Overview) != "" {
			merged.Overview = headline.Overview
		}
	}
	if err := ValidateSummaryResult(merged); err != nil {
		return nil, err
	}
	return merged, nil
}

type summaryHeadline struct {
	Title    string `json:"title"`
	Overview string `json:"overview"`
}

func (c *OpenAICompatible) summarizeHeadline(ctx context.Context, input SummaryInput, topics []SummaryTopicResult) (*summaryHeadline, error) {
	type headlineTopic struct {
		Title        string `json:"title"`
		Category     string `json:"category"`
		ShortSummary string `json:"short_summary"`
	}
	headlineTopics := make([]headlineTopic, 0, len(topics))
	for _, topic := range topics {
		headlineTopics = append(headlineTopics, headlineTopic{
			Title:        topic.Title,
			Category:     topic.Category,
			ShortSummary: topic.ShortSummary,
		})
	}
	payload := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: headlineSystemPrompt},
			{Role: "user", Content: mustJSON(map[string]any{"language": input.Language, "topics": headlineTopics})},
		},
		Temperature:    0.2,
		ResponseFormat: map[string]string{"type": "json_object"},
	}
	raw, err := c.doChat(ctx, payload)
	if err != nil {
		return nil, err
	}
	var headline summaryHeadline
	if err := json.Unmarshal(raw, &headline); err != nil {
		return nil, fmt.Errorf("parse summary headline json: %w", err)
	}
	return &headline, nil
}

const headlineSystemPrompt = `Тебе дан список тем уже готовой сводки Telegram.
Составь общий заголовок и краткий обзор на русском языке, опираясь только на переданные темы.
Не выдумывай факты и не добавляй темы, которых нет в списке.
Верни только JSON: {"title":"string","overview":"string"}.`
