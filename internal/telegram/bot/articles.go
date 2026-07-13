package bot

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/kirilllebedenko/content_scout/internal/article"
	"github.com/kirilllebedenko/content_scout/internal/domain"
)

type ArticleController interface {
	ConvertSummary(ctx context.Context, req article.ConvertRequest) (*article.Result, error)
	ConvertTopic(ctx context.Context, req article.ConvertRequest) (*article.Result, error)
	List(ctx context.Context, telegramUserID int64, limit int) ([]domain.Article, error)
	Get(ctx context.Context, telegramUserID, articleID int64) (*domain.Article, error)
	UpdateMetadata(ctx context.Context, telegramUserID, articleID int64, title string, tags []string) (*domain.Article, error)
}

func (r *Router) createArticleFromSummary(ctx context.Context, chatID, userID int64, args string, editMessageID int, callbackAnswer string) (Outgoing, error) {
	fields := strings.Fields(args)
	if len(fields) < 1 {
		return Outgoing{ChatID: chatID, Text: "Использование: /article_from_summary <summary_id> [analysis|guide|educational|outline|telegram_post]", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	summaryID, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil || summaryID <= 0 {
		return Outgoing{ChatID: chatID, Text: "summary_id должен быть положительным числом.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	articleType := domain.ArticleType("")
	if len(fields) > 1 {
		articleType = domain.ArticleType(fields[1])
	}
	return r.convertArticleSummary(ctx, chatID, userID, summaryID, articleType, editMessageID, callbackAnswer)
}

func (r *Router) createArticleFromTopic(ctx context.Context, chatID, userID int64, args string, editMessageID int, callbackAnswer string) (Outgoing, error) {
	fields := strings.Fields(args)
	if len(fields) < 2 {
		return Outgoing{ChatID: chatID, Text: "Использование: /article_from_topic <summary_id> <position> [analysis|guide|educational|outline|telegram_post]", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	summaryID, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil || summaryID <= 0 {
		return Outgoing{ChatID: chatID, Text: "summary_id должен быть положительным числом.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	position, err := strconv.Atoi(fields[1])
	if err != nil || position <= 0 {
		return Outgoing{ChatID: chatID, Text: "position должен быть положительным числом.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	articleType := domain.ArticleType("")
	if len(fields) > 2 {
		articleType = domain.ArticleType(fields[2])
	}
	return r.convertArticleTopic(ctx, chatID, userID, summaryID, position, articleType, editMessageID, callbackAnswer)
}

func (r *Router) showArticles(ctx context.Context, chatID, userID int64, editMessageID int, callbackAnswer string) (Outgoing, error) {
	if r.articles == nil {
		return Outgoing{ChatID: chatID, Text: "Конвертация статей пока не настроена.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	items, err := r.articles.List(ctx, userID, 10)
	if err != nil {
		return Outgoing{ChatID: chatID, Text: publicAuthError(err), Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	if err := r.states.Set(ctx, userID, DialogState{View: ViewArticles}); err != nil {
		return Outgoing{}, fmt.Errorf("set dialog state: %w", err)
	}
	if len(items) == 0 {
		return Outgoing{ChatID: chatID, Text: "Черновиков статей пока нет.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	return Outgoing{ChatID: chatID, Text: articlesListText(items), Menu: articlesMenu(items), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
}

func (r *Router) showArticle(ctx context.Context, chatID, userID int64, args string, editMessageID int, callbackAnswer string) (Outgoing, error) {
	articleID, err := parseRequiredInt64(args)
	if err != nil {
		return Outgoing{ChatID: chatID, Text: "Использование: /article <article_id>", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	return r.renderArticle(ctx, chatID, userID, articleID, editMessageID, callbackAnswer)
}

func (r *Router) renameArticle(ctx context.Context, chatID, userID int64, args string, editMessageID int, callbackAnswer string) (Outgoing, error) {
	fields := strings.Fields(args)
	if len(fields) < 2 {
		return Outgoing{ChatID: chatID, Text: "Использование: /article_title <article_id> <новое название>", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	articleID, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil || articleID <= 0 {
		return Outgoing{ChatID: chatID, Text: "article_id должен быть положительным числом.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	if r.articles == nil {
		return Outgoing{ChatID: chatID, Text: "Конвертация статей пока не настроена.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	updated, err := r.articles.UpdateMetadata(ctx, userID, articleID, strings.Join(fields[1:], " "), nil)
	if errors.Is(err, article.ErrArticleNotFound) {
		return Outgoing{ChatID: chatID, Text: "Статья не найдена.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	if err != nil {
		return Outgoing{ChatID: chatID, Text: publicAuthError(err), Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	return Outgoing{ChatID: chatID, Text: articleText(*updated), Menu: articleMenu(updated.ID), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
}

func (r *Router) updateArticleTags(ctx context.Context, chatID, userID int64, args string, editMessageID int, callbackAnswer string) (Outgoing, error) {
	fields := strings.Fields(args)
	if len(fields) < 2 {
		return Outgoing{ChatID: chatID, Text: "Использование: /article_tags <article_id> tag1,tag2", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	articleID, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil || articleID <= 0 {
		return Outgoing{ChatID: chatID, Text: "article_id должен быть положительным числом.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	if r.articles == nil {
		return Outgoing{ChatID: chatID, Text: "Конвертация статей пока не настроена.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	tags := strings.FieldsFunc(strings.Join(fields[1:], " "), func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\t' })
	updated, err := r.articles.UpdateMetadata(ctx, userID, articleID, "", tags)
	if errors.Is(err, article.ErrArticleNotFound) {
		return Outgoing{ChatID: chatID, Text: "Статья не найдена.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	if err != nil {
		return Outgoing{ChatID: chatID, Text: publicAuthError(err), Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	return Outgoing{ChatID: chatID, Text: articleText(*updated), Menu: articleMenu(updated.ID), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
}

func (r *Router) handleArticleCallback(ctx context.Context, in Incoming) (Outgoing, error) {
	fields := strings.Split(in.CallbackData, ":")
	if len(fields) < 2 {
		return Outgoing{ChatID: in.ChatID, Text: "Неизвестное действие.", AnswerCallback: "Неизвестное действие."}, nil
	}
	switch fields[1] {
	case "list":
		return r.showArticles(ctx, in.ChatID, in.UserID, in.CallbackMessage, "Статьи открыты.")
	case "open":
		if len(fields) != 3 {
			return Outgoing{ChatID: in.ChatID, Text: "Неизвестное действие.", AnswerCallback: "Неизвестное действие."}, nil
		}
		articleID, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil || articleID <= 0 {
			return Outgoing{ChatID: in.ChatID, Text: "Неизвестная статья.", AnswerCallback: "Неизвестная статья."}, nil
		}
		return r.renderArticle(ctx, in.ChatID, in.UserID, articleID, in.CallbackMessage, "Статья открыта.")
	case "summary":
		if len(fields) != 3 {
			return Outgoing{ChatID: in.ChatID, Text: "Неизвестное действие.", AnswerCallback: "Неизвестное действие."}, nil
		}
		summaryID, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil || summaryID <= 0 {
			return Outgoing{ChatID: in.ChatID, Text: "Неизвестное summary.", AnswerCallback: "Неизвестное summary."}, nil
		}
		return r.convertArticleSummary(ctx, in.ChatID, in.UserID, summaryID, "", in.CallbackMessage, "Создаю статью.")
	case "topic":
		if len(fields) != 4 {
			return Outgoing{ChatID: in.ChatID, Text: "Неизвестное действие.", AnswerCallback: "Неизвестное действие."}, nil
		}
		summaryID, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil || summaryID <= 0 {
			return Outgoing{ChatID: in.ChatID, Text: "Неизвестное summary.", AnswerCallback: "Неизвестное summary."}, nil
		}
		position, err := strconv.Atoi(fields[3])
		if err != nil || position <= 0 {
			return Outgoing{ChatID: in.ChatID, Text: "Неизвестная тема.", AnswerCallback: "Неизвестная тема."}, nil
		}
		return r.convertArticleTopic(ctx, in.ChatID, in.UserID, summaryID, position, "", in.CallbackMessage, "Создаю статью.")
	default:
		return Outgoing{ChatID: in.ChatID, Text: "Неизвестное действие.", AnswerCallback: "Неизвестное действие."}, nil
	}
}

func (r *Router) convertArticleSummary(ctx context.Context, chatID, userID, summaryID int64, articleType domain.ArticleType, editMessageID int, callbackAnswer string) (Outgoing, error) {
	if r.articles == nil {
		return Outgoing{ChatID: chatID, Text: "Конвертация статей пока не настроена.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	result, err := r.articles.ConvertSummary(ctx, article.ConvertRequest{TelegramUserID: userID, SummaryID: summaryID, Type: articleType})
	return articleResultOutgoing(chatID, editMessageID, callbackAnswer, result, err)
}

func (r *Router) convertArticleTopic(ctx context.Context, chatID, userID, summaryID int64, position int, articleType domain.ArticleType, editMessageID int, callbackAnswer string) (Outgoing, error) {
	if r.articles == nil {
		return Outgoing{ChatID: chatID, Text: "Конвертация статей пока не настроена.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	result, err := r.articles.ConvertTopic(ctx, article.ConvertRequest{TelegramUserID: userID, SummaryID: summaryID, TopicPosition: position, Type: articleType})
	return articleResultOutgoing(chatID, editMessageID, callbackAnswer, result, err)
}

func (r *Router) renderArticle(ctx context.Context, chatID, userID, articleID int64, editMessageID int, callbackAnswer string) (Outgoing, error) {
	if r.articles == nil {
		return Outgoing{ChatID: chatID, Text: "Конвертация статей пока не настроена.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	item, err := r.articles.Get(ctx, userID, articleID)
	if errors.Is(err, article.ErrArticleNotFound) {
		return Outgoing{ChatID: chatID, Text: "Статья не найдена.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	if err != nil {
		return Outgoing{ChatID: chatID, Text: publicAuthError(err), Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	return Outgoing{ChatID: chatID, Text: articleText(*item), Menu: articleMenu(item.ID), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
}

func articleResultOutgoing(chatID int64, editMessageID int, callbackAnswer string, result *article.Result, err error) (Outgoing, error) {
	if errors.Is(err, article.ErrArticleNotFound) {
		return Outgoing{ChatID: chatID, Text: "Статья не найдена.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	if err != nil {
		return Outgoing{ChatID: chatID, Text: publicAuthError(err), Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	return Outgoing{ChatID: chatID, Text: articleCreatedText(result), Menu: articleMenu(result.Article.ID), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
}

func articlesListText(items []domain.Article) string {
	var b strings.Builder
	b.WriteString("Черновики статей\n")
	for _, item := range items {
		fmt.Fprintf(&b, "\n#%d %s\n%s | %s | tags: %s", item.ID, fallbackTitle(item.Title), item.Type, item.Status, tagsText(item.Tags))
	}
	return b.String()
}

func articleText(item domain.Article) string {
	return fmt.Sprintf("#%d %s\n\nSlug: %s\nТип: %s\nСтатус: %s\nTags: %s\n\n%s", item.ID, fallbackTitle(item.Title), item.Slug, item.Type, item.Status, tagsText(item.Tags), item.ContentMarkdown)
}

func articleCreatedText(result *article.Result) string {
	return fmt.Sprintf("Черновик статьи создан.\n\n#%d %s\nSlug: %s\nТип: %s\nИсточников: %d\nTags: %s", result.Article.ID, fallbackTitle(result.Article.Title), result.Article.Slug, result.Article.Type, result.Sources, tagsText(result.Article.Tags))
}

func articlesMenu(items []domain.Article) Menu {
	menu := make(Menu, 0, len(items)+1)
	for _, item := range items {
		menu = append(menu, []MenuButton{{Text: fmt.Sprintf("#%d %s", item.ID, compactButtonTitle(item.Title)), Data: fmt.Sprintf("art:open:%d", item.ID)}})
	}
	menu = append(menu, []MenuButton{{Text: "Назад", Data: ActionBackHome}})
	return menu
}

func articleMenu(articleID int64) Menu {
	return Menu{
		{{Text: "Экспорт", Data: fmt.Sprintf("exp:article:%d", articleID)}},
		{{Text: "К статьям", Data: "art:list"}, {Text: "Назад", Data: ActionBackHome}},
		{{Text: fmt.Sprintf("ID %d", articleID), Data: "art:list"}},
	}
}

func tagsText(tags []string) string {
	if len(tags) == 0 {
		return "-"
	}
	return strings.Join(tags, ", ")
}
