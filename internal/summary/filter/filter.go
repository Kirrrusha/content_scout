package filter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"unicode"

	"github.com/kirilllebedenko/content_scout/internal/domain"
)

type Rules struct {
	MinTextLength int
	DropAds       bool
	DropJobs      bool
}

type Stats struct {
	InputCount         int
	EmptyCount         int
	TooShortCount      int
	EmojiOnlyCount     int
	AdvertisementCount int
	JobPostCount       int
	FooterRemovedCount int
	KeptCount          int
}

type Message struct {
	Source      domain.CollectedMessage
	Content     string
	URLs        []string
	ContentHash string
}

type Filter struct{}

func New() *Filter {
	return &Filter{}
}

func (f *Filter) Filter(_ context.Context, messages []domain.CollectedMessage, rules Rules) ([]Message, Stats, error) {
	if rules.MinTextLength <= 0 {
		rules.MinTextLength = 30
	}
	stats := Stats{InputCount: len(messages)}
	filtered := make([]Message, 0, len(messages))
	for _, source := range messages {
		content, footerRemoved := Normalize(source.Text, source.Caption)
		if footerRemoved {
			stats.FooterRemovedCount++
		}
		urls := URLs(content + " " + source.URL)
		if strings.TrimSpace(content) == "" {
			stats.EmptyCount++
			continue
		}
		if emojiOnly(content) {
			stats.EmojiOnlyCount++
			continue
		}
		if len([]rune(content)) < rules.MinTextLength && len(urls) == 0 {
			stats.TooShortCount++
			continue
		}
		if rules.DropAds && looksLikeAd(content) {
			stats.AdvertisementCount++
			continue
		}
		if rules.DropJobs && looksLikeJob(content) {
			stats.JobPostCount++
			continue
		}
		hash := sha256.Sum256([]byte(canonical(content)))
		filtered = append(filtered, Message{
			Source:      source,
			Content:     content,
			URLs:        urls,
			ContentHash: hex.EncodeToString(hash[:]),
		})
	}
	stats.KeptCount = len(filtered)
	return filtered, stats, nil
}

var (
	urlPattern    = regexp.MustCompile(`https?://[^\s)>\]]+|t\.me/[^\s)>\]]+`)
	spacePattern  = regexp.MustCompile(`[ \t]+`)
	footerPattern = regexp.MustCompile(`(?i)(подписаться|читать далее|источник|наш канал)\s*[:\-—]?.*$`)
)

func Normalize(text, caption string) (string, bool) {
	content := strings.TrimSpace(strings.Join(nonEmpty(text, caption), "\n\n"))
	if content == "" {
		return "", false
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	normalized := make([]string, 0, len(lines))
	footerRemoved := false
	for _, line := range lines {
		line = strings.TrimSpace(spacePattern.ReplaceAllString(line, " "))
		if line == "" {
			if len(normalized) > 0 && normalized[len(normalized)-1] != "" {
				normalized = append(normalized, "")
			}
			continue
		}
		if footerPattern.MatchString(line) && len([]rune(line)) < 120 {
			footerRemoved = true
			continue
		}
		normalized = append(normalized, line)
	}
	return strings.TrimSpace(strings.Join(normalized, "\n")), footerRemoved
}

func URLs(content string) []string {
	raw := urlPattern.FindAllString(content, -1)
	seen := make(map[string]struct{}, len(raw))
	urls := make([]string, 0, len(raw))
	for _, value := range raw {
		value = strings.TrimRight(value, ".,;:")
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		urls = append(urls, value)
	}
	return urls
}

func canonical(content string) string {
	return strings.ToLower(spacePattern.ReplaceAllString(strings.TrimSpace(content), " "))
}

func nonEmpty(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func emojiOnly(content string) bool {
	hasLetterOrDigit := false
	hasOtherSignal := false
	for _, r := range content {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			hasLetterOrDigit = true
		case unicode.IsSpace(r), unicode.IsPunct(r), unicode.IsSymbol(r):
		default:
			hasOtherSignal = true
		}
	}
	return !hasLetterOrDigit && !hasOtherSignal
}

func looksLikeAd(content string) bool {
	lowered := strings.ToLower(content)
	markers := []string{"реклама", "промокод", "скидка", "партнерский", "sponsored", "ad:"}
	for _, marker := range markers {
		if strings.Contains(lowered, marker) {
			return true
		}
	}
	return false
}

func looksLikeJob(content string) bool {
	lowered := strings.ToLower(content)
	markers := []string{"вакансия", "ищем разработчика", "зарплата", "удаленка", "remote job", "hiring"}
	for _, marker := range markers {
		if strings.Contains(lowered, marker) {
			return true
		}
	}
	return false
}
