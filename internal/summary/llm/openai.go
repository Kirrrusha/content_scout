package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
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
	ctx, cancel := c.withRequestBudget(ctx)
	defer cancel()

	payload := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: summarySystemPrompt},
			{Role: "user", Content: mustJSON(input)},
		},
		Temperature:         completionTemperature(c.model, 0.2),
		MaxCompletionTokens: maxSummaryCompletionTokens,
		ResponseFormat:      map[string]string{"type": "json_object"},
		ChatTemplateKwargs:  instantModeTemplateArgs(c.model),
	}
	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		raw, err := c.doChat(ctx, payload)
		if err != nil {
			lastErr = err
			if ctx.Err() != nil || !isRetryableLLMError(err) {
				break
			}
			sleepBackoff(ctx, attempt)
			continue
		}
		result, err := ParseSummaryResult(raw)
		if err != nil {
			lastErr = err
			if ctx.Err() != nil {
				break
			}
			sleepBackoff(ctx, attempt)
			continue
		}
		normalizeSummarySourceIndexes(result)
		return result, nil
	}
	return nil, fmt.Errorf("summarize with llm: %w", lastErr)
}

// withRequestBudget applies the configured HTTP timeout to the complete logical
// request, including retries. http.Client.Timeout alone restarts for every retry,
// so retries: 2 could otherwise turn a three-minute batch into a nine-minute one.
func (c *OpenAICompatible) withRequestBudget(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.client == nil || c.client.Timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, c.client.Timeout)
}

func (c *OpenAICompatible) ConvertToArticle(ctx context.Context, input ArticleInput) (*ArticleResult, error) {
	if c.apiKey == "" {
		return nil, errors.New("LLM_API_KEY is not configured")
	}
	if c.model == "" {
		return nil, errors.New("LLM_MODEL is not configured")
	}
	ctx, cancel := c.withRequestBudget(ctx)
	defer cancel()

	payload := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: articleSystemPrompt},
			{Role: "user", Content: mustJSON(input)},
		},
		Temperature:         completionTemperature(c.model, 0.25),
		MaxCompletionTokens: maxArticleCompletionTokens,
		ResponseFormat:      map[string]string{"type": "json_object"},
		ChatTemplateKwargs:  instantModeTemplateArgs(c.model),
	}
	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		raw, err := c.doChat(ctx, payload)
		if err != nil {
			lastErr = err
			if ctx.Err() != nil || !isRetryableLLMError(err) {
				break
			}
			sleepBackoff(ctx, attempt)
			continue
		}
		result, err := ParseArticleResult(raw)
		if err != nil {
			lastErr = err
			if ctx.Err() != nil {
				break
			}
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
		return nil, retryableLLMError{err: fmt.Errorf("llm temporary status: %d", resp.StatusCode)}
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

type retryableLLMError struct {
	err error
}

func (e retryableLLMError) Error() string { return e.err.Error() }
func (e retryableLLMError) Unwrap() error { return e.err }

func isRetryableLLMError(err error) bool {
	var retryable retryableLLMError
	if errors.As(err, &retryable) {
		return true
	}
	var networkErr net.Error
	return errors.As(err, &networkErr) && !networkErr.Timeout()
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
	Model               string            `json:"model"`
	Messages            []chatMessage     `json:"messages"`
	Temperature         *float64          `json:"temperature,omitempty"`
	MaxCompletionTokens int               `json:"max_completion_tokens,omitempty"`
	ResponseFormat      map[string]string `json:"response_format,omitempty"`
	ChatTemplateKwargs  map[string]any    `json:"chat_template_kwargs,omitempty"`
}

const (
	maxSummaryCompletionTokens  = 3500
	maxArticleCompletionTokens  = 6000
	maxHeadlineCompletionTokens = 800
	maxReduceCompletionTokens   = 2000
)

// Kimi K2.5/K2.6 enable reasoning by default. Summary generation is a bounded
// transformation task, and in production the model repeatedly spent the whole
// request timeout on reasoning without emitting any JSON. Cloud.ru exposes the
// model's instant mode through the vLLM chat template arguments.
func instantModeTemplateArgs(model string) map[string]any {
	normalized := strings.ToLower(model)
	if strings.Contains(normalized, "kimi-k2.5") || strings.Contains(normalized, "kimi-k2.6") {
		return map[string]any{"thinking": false}
	}
	return nil
}

func completionTemperature(model string, value float64) *float64 {
	if instantModeTemplateArgs(model) != nil {
		return nil
	}
	return &value
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
	// maxReduceContentBytes bounds each cross-batch topic clustering request.
	// Unlike batch summarization, reduce only sends compact topic descriptors and
	// never repeats the original Telegram messages.
	maxReduceContentBytes = 48000
	maxReduceTitleBytes   = 240
	maxReduceSummaryBytes = 700
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
		message = limitSummaryMessageSize(message, maxBytes)
		messageSize := len(message.Text) + len(message.ChatTitle)
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

func limitSummaryMessageSize(message SummaryMessageInput, maxBytes int) SummaryMessageInput {
	if maxBytes <= 0 {
		return message
	}
	if len(message.ChatTitle) >= maxBytes {
		message.ChatTitle = truncateUTF8(message.ChatTitle, maxBytes)
		message.Text = ""
		return message
	}
	message.Text = truncateUTF8(message.Text, maxBytes-len(message.ChatTitle))
	return message
}

func truncateUTF8(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	for maxBytes > 0 && !utf8.RuneStart(value[maxBytes]) {
		maxBytes--
	}
	return value[:maxBytes]
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
			select {
			case semaphore <- struct{}{}:
			case <-ctx.Done():
				errs[i] = ctx.Err()
				return
			}
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
		result.Topics[i].SourceIndexes = uniqueSortedIndexes(mapped)
	}
}

func normalizeSummarySourceIndexes(result *SummaryResult) {
	for i := range result.Topics {
		result.Topics[i].SourceIndexes = uniqueSortedIndexes(result.Topics[i].SourceIndexes)
	}
}

// mergeSummaries collects the batch topics, semantically merges duplicates using
// compact topic descriptors, then asks for one title and overview. Original
// message text is never repeated during reduce. Every reduce call is optional:
// partial or total failure still returns the unmerged, usable batch summaries.
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
	merged.Topics = c.mergeDuplicateTopics(ctx, merged.Topics)
	if headline, err := c.summarizeHeadline(ctx, input, merged.Topics); err == nil {
		if strings.TrimSpace(headline.Title) != "" {
			merged.Title = headline.Title
		}
		if strings.TrimSpace(headline.Overview) != "" {
			merged.Overview = headline.Overview
		}
	} else {
		c.logger.Warn("summary headline reduce failed; using batch fallback", "error", err)
	}
	if err := ValidateSummaryResult(merged); err != nil {
		return nil, err
	}
	return merged, nil
}

type reduceTopic struct {
	Index        int    `json:"index"`
	Title        string `json:"title"`
	Category     string `json:"category,omitempty"`
	ShortSummary string `json:"short_summary"`
}

type topicMerge struct {
	TopicIndexes []int  `json:"topic_indexes"`
	SharedAnchor string `json:"shared_anchor"`
	Title        string `json:"title"`
	ShortSummary string `json:"short_summary"`
}

type topicMergeResult struct {
	Merges []topicMerge `json:"merges"`
}

// mergeDuplicateTopics runs a global reduce for ordinary digests. For unusually
// large topic sets it uses bounded chunks and a second title-sorted pass, which
// gives duplicates separated by batch boundaries another chance to meet without
// allowing a request to grow without bound.
func (c *OpenAICompatible) mergeDuplicateTopics(ctx context.Context, topics []SummaryTopicResult) []SummaryTopicResult {
	if len(topics) < 2 {
		return topics
	}
	current := topics
	for pass := 0; pass < 2; pass++ {
		order := make([]int, len(current))
		for i := range order {
			order[i] = i
		}
		if pass == 1 {
			sort.SliceStable(order, func(i, j int) bool {
				left := strings.ToLower(current[order[i]].Category + " " + current[order[i]].Title)
				right := strings.ToLower(current[order[j]].Category + " " + current[order[j]].Title)
				return left < right
			})
		}
		chunks := splitReduceTopics(current, order, maxReduceContentBytes)
		var passMerges []topicMerge
		for _, chunk := range chunks {
			result, err := c.reduceTopicChunk(ctx, current, chunk)
			if err != nil {
				c.logger.Warn("summary topic reduce chunk failed; keeping original topics", "topics", len(chunk), "error", err)
				continue
			}
			passMerges = append(passMerges, result.Merges...)
		}
		current = applyTopicMerges(current, passMerges)
		if len(chunks) == 1 {
			break
		}
	}
	return current
}

func splitReduceTopics(topics []SummaryTopicResult, order []int, maxBytes int) [][]int {
	var chunks [][]int
	var chunk []int
	size := 0
	for _, index := range order {
		descriptor := compactReduceTopic(index, topics[index])
		descriptorSize := len(mustJSON(descriptor)) + 1
		if len(chunk) > 0 && size+descriptorSize > maxBytes {
			chunks = append(chunks, chunk)
			chunk = nil
			size = 0
		}
		chunk = append(chunk, index)
		size += descriptorSize
	}
	if len(chunk) > 0 {
		chunks = append(chunks, chunk)
	}
	return chunks
}

func compactReduceTopic(index int, topic SummaryTopicResult) reduceTopic {
	return reduceTopic{
		Index:        index,
		Title:        truncateUTF8(topic.Title, maxReduceTitleBytes),
		Category:     truncateUTF8(topic.Category, maxReduceTitleBytes),
		ShortSummary: truncateUTF8(topic.ShortSummary, maxReduceSummaryBytes),
	}
}

func (c *OpenAICompatible) reduceTopicChunk(ctx context.Context, topics []SummaryTopicResult, indexes []int) (*topicMergeResult, error) {
	descriptors := make([]reduceTopic, 0, len(indexes))
	for _, index := range indexes {
		descriptors = append(descriptors, compactReduceTopic(index, topics[index]))
	}
	ctx, cancel := c.withRequestBudget(ctx)
	defer cancel()
	payload := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: topicReduceSystemPrompt},
			{Role: "user", Content: mustJSON(map[string]any{"topics": descriptors})},
		},
		Temperature:         completionTemperature(c.model, 0.1),
		MaxCompletionTokens: maxReduceCompletionTokens,
		ResponseFormat:      map[string]string{"type": "json_object"},
		ChatTemplateKwargs:  instantModeTemplateArgs(c.model),
	}
	raw, err := c.doChat(ctx, payload)
	if err != nil {
		return nil, err
	}
	var result topicMergeResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse topic reduce json: %w", err)
	}
	allowed := make(map[int]struct{}, len(indexes))
	for _, index := range indexes {
		allowed[index] = struct{}{}
	}
	valid := result.Merges[:0]
	for _, merge := range result.Merges {
		insideChunk := true
		for _, index := range merge.TopicIndexes {
			if _, ok := allowed[index]; !ok {
				insideChunk = false
				break
			}
		}
		if insideChunk && hasConcreteSharedAnchor(topics, merge) {
			valid = append(valid, merge)
		}
	}
	result.Merges = valid
	return &result, nil
}

func hasConcreteSharedAnchor(topics []SummaryTopicResult, merge topicMerge) bool {
	anchor := strings.ToLower(strings.TrimSpace(merge.SharedAnchor))
	if utf8.RuneCountInString(anchor) < 4 || genericMergeAnchor(anchor) {
		return false
	}
	for _, index := range merge.TopicIndexes {
		if index < 0 || index >= len(topics) {
			return false
		}
		// The short summary helps the model decide whether two topics describe the
		// same story, but accepting an anchor from prose is too permissive: generic
		// phrases such as "short post" can occur in unrelated summaries. A concrete
		// entity must be named in every topic title to pass the local safety check.
		haystack := strings.ToLower(topics[index].Title)
		if !strings.Contains(haystack, anchor) {
			return false
		}
	}
	return true
}

func genericMergeAnchor(anchor string) bool {
	genericPrefixes := []string{
		"верси", "видео", "выпуск", "интернет", "компан", "контент", "культур",
		"мем", "модел", "нов", "обновлен", "пост", "проект", "релиз",
		"сарказм", "сатир", "сериал", "сообщен", "тем", "юмор",
	}
	words := strings.FieldsFunc(anchor, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	if len(words) == 0 {
		return true
	}
	for _, word := range words {
		isGeneric := false
		for _, prefix := range genericPrefixes {
			if strings.HasPrefix(word, prefix) {
				isGeneric = true
				break
			}
		}
		if !isGeneric {
			return false
		}
	}
	return true
}

func applyTopicMerges(topics []SummaryTopicResult, merges []topicMerge) []SummaryTopicResult {
	groups := make(map[int]topicMerge)
	members := make(map[int]int)
	for _, merge := range merges {
		valid := uniqueSortedIndexes(merge.TopicIndexes)
		if len(valid) < 2 {
			continue
		}
		validGroup := true
		for _, index := range valid {
			if index < 0 || index >= len(topics) {
				validGroup = false
				break
			}
			if _, exists := members[index]; exists {
				validGroup = false
				break
			}
		}
		if !validGroup {
			continue
		}
		root := valid[0]
		merge.TopicIndexes = valid
		groups[root] = merge
		for _, index := range valid {
			members[index] = root
		}
	}
	if len(groups) == 0 {
		return topics
	}
	result := make([]SummaryTopicResult, 0, len(topics)-len(members)+len(groups))
	for index, topic := range topics {
		root, merged := members[index]
		if !merged {
			topic.SourceIndexes = uniqueSortedIndexes(topic.SourceIndexes)
			result = append(result, topic)
			continue
		}
		if root != index {
			continue
		}
		result = append(result, combineTopicGroup(topics, groups[root]))
	}
	return result
}

func combineTopicGroup(topics []SummaryTopicResult, merge topicMerge) SummaryTopicResult {
	representative := topics[merge.TopicIndexes[0]]
	var sources []int
	var fullSummaries, reasons []string
	for _, index := range merge.TopicIndexes {
		topic := topics[index]
		sources = append(sources, topic.SourceIndexes...)
		fullSummaries = appendUniqueText(fullSummaries, topic.FullSummary)
		reasons = appendUniqueText(reasons, topic.WhyImportant)
		if topic.Importance > representative.Importance {
			representative.Importance = topic.Importance
		}
		representative.Confidence = lowerConfidence(representative.Confidence, topic.Confidence)
	}
	if strings.TrimSpace(merge.Title) != "" {
		representative.Title = strings.TrimSpace(merge.Title)
	}
	if strings.TrimSpace(merge.ShortSummary) != "" {
		representative.ShortSummary = strings.TrimSpace(merge.ShortSummary)
	}
	representative.FullSummary = strings.Join(fullSummaries, "\n\n")
	representative.WhyImportant = strings.Join(reasons, "\n\n")
	representative.SourceIndexes = uniqueSortedIndexes(sources)
	return representative
}

func appendUniqueText(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if strings.EqualFold(existing, value) {
			return values
		}
	}
	return append(values, value)
}

func uniqueSortedIndexes(indexes []int) []int {
	seen := make(map[int]struct{}, len(indexes))
	unique := make([]int, 0, len(indexes))
	for _, index := range indexes {
		if _, ok := seen[index]; ok {
			continue
		}
		seen[index] = struct{}{}
		unique = append(unique, index)
	}
	sort.Ints(unique)
	return unique
}

func lowerConfidence(left, right string) string {
	rank := map[string]int{"low": 1, "medium": 2, "high": 3}
	if rank[right] < rank[left] {
		return right
	}
	return left
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
	if len(mustJSON(headlineTopics)) > maxReduceContentBytes {
		return nil, errors.New("summary headline input exceeds reduce budget")
	}
	ctx, cancel := c.withRequestBudget(ctx)
	defer cancel()
	payload := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: headlineSystemPrompt},
			{Role: "user", Content: mustJSON(map[string]any{"language": input.Language, "topics": headlineTopics})},
		},
		Temperature:         completionTemperature(c.model, 0.2),
		MaxCompletionTokens: maxHeadlineCompletionTokens,
		ResponseFormat:      map[string]string{"type": "json_object"},
		ChatTemplateKwargs:  instantModeTemplateArgs(c.model),
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

const topicReduceSystemPrompt = `Тебе дан список тем из разных батчей одной Telegram-сводки. Для каждой темы переданы только индекс, заголовок, категория и краткое описание; исходных сообщений нет.
Найди только семантически одинаковые темы об одном и том же событии, вопросе или сюжете. Не объединяй темы лишь из-за общей категории, похожих слов, одного человека или одной компании. Если связь неочевидна, оставь темы независимыми.
Для каждой подтвержденной группы дублей верни все исходные индексы ровно один раз, общий точный заголовок и краткое описание. Также верни shared_anchor — название одной конкретной сущности, события, продукта, человека или места, которое дословно присутствует в заголовке каждой темы группы. Общая категория, стиль, источник, слово "новости", "пост", "релиз", "модель" или "интернет" не являются допустимым anchor. Если такого конкретного общего anchor нет во всех заголовках, не объединяй темы.
Индексы разных групп не должны пересекаться. Независимые темы не возвращай.
Верни только JSON: {"merges":[{"topic_indexes":[0,3],"shared_anchor":"string","title":"string","short_summary":"string"}]}.`
