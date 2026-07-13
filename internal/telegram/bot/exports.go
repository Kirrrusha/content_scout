package bot

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/kirilllebedenko/content_scout/internal/article"
	"github.com/kirilllebedenko/content_scout/internal/obsidian"
	"github.com/kirilllebedenko/content_scout/internal/summary"
)

type ExportController interface {
	ExportArticle(ctx context.Context, telegramUserID, articleID int64) (*obsidian.Result, error)
	ExportSummary(ctx context.Context, telegramUserID, summaryID int64) (*obsidian.Result, error)
}

func (r *Router) exportArticle(ctx context.Context, chatID, userID int64, args string, editMessageID int, callbackAnswer string) (Outgoing, error) {
	articleID, err := parseRequiredInt64(args)
	if err != nil {
		return Outgoing{ChatID: chatID, Text: "Использование: /export_article <article_id>", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	return r.renderArticleExport(ctx, chatID, userID, articleID, editMessageID, callbackAnswer)
}

func (r *Router) exportSummary(ctx context.Context, chatID, userID int64, args string, editMessageID int, callbackAnswer string) (Outgoing, error) {
	summaryID, err := parseRequiredInt64(args)
	if err != nil {
		return Outgoing{ChatID: chatID, Text: "Использование: /export_summary <summary_id>", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	return r.renderSummaryExport(ctx, chatID, userID, summaryID, editMessageID, callbackAnswer)
}

func (r *Router) handleExportCallback(ctx context.Context, in Incoming) (Outgoing, error) {
	fields := strings.Split(in.CallbackData, ":")
	if len(fields) != 3 {
		return Outgoing{ChatID: in.ChatID, Text: "Неизвестное действие.", AnswerCallback: "Неизвестное действие."}, nil
	}
	id, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil || id <= 0 {
		return Outgoing{ChatID: in.ChatID, Text: "Неизвестный экспорт.", AnswerCallback: "Неизвестный экспорт."}, nil
	}
	switch fields[1] {
	case "article":
		return r.renderArticleExport(ctx, in.ChatID, in.UserID, id, 0, "Экспорт готов.")
	case "summary":
		return r.renderSummaryExport(ctx, in.ChatID, in.UserID, id, 0, "Экспорт готов.")
	default:
		return Outgoing{ChatID: in.ChatID, Text: "Неизвестное действие.", AnswerCallback: "Неизвестное действие."}, nil
	}
}

func (r *Router) renderArticleExport(ctx context.Context, chatID, userID, articleID int64, editMessageID int, callbackAnswer string) (Outgoing, error) {
	if r.exports == nil {
		return Outgoing{ChatID: chatID, Text: "Экспорт в Obsidian пока не настроен.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	result, err := r.exports.ExportArticle(ctx, userID, articleID)
	return exportOutgoing(chatID, editMessageID, callbackAnswer, result, err)
}

func (r *Router) renderSummaryExport(ctx context.Context, chatID, userID, summaryID int64, editMessageID int, callbackAnswer string) (Outgoing, error) {
	if r.exports == nil {
		return Outgoing{ChatID: chatID, Text: "Экспорт в Obsidian пока не настроен.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	result, err := r.exports.ExportSummary(ctx, userID, summaryID)
	return exportOutgoing(chatID, editMessageID, callbackAnswer, result, err)
}

func exportOutgoing(chatID int64, editMessageID int, callbackAnswer string, result *obsidian.Result, err error) (Outgoing, error) {
	if errors.Is(err, article.ErrArticleNotFound) {
		return Outgoing{ChatID: chatID, Text: "Статья не найдена.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	if errors.Is(err, summary.ErrSummaryNotFound) {
		return Outgoing{ChatID: chatID, Text: "Summary не найдено.", Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	if err != nil {
		return Outgoing{ChatID: chatID, Text: publicAuthError(err), Menu: BackMenu(), EditMessageID: editMessageID, AnswerCallback: callbackAnswer}, nil
	}
	text := fmt.Sprintf("Markdown экспортирован для Obsidian.\n\n%s", result.Export.VaultPath)
	if result.Reused {
		text = fmt.Sprintf("Markdown уже экспортирован ранее.\n\n%s", result.Export.VaultPath)
	}
	return Outgoing{
		ChatID:         chatID,
		Text:           text,
		Menu:           BackMenu(),
		DocumentPath:   result.Path,
		DocumentName:   result.Export.FileName,
		EditMessageID:  editMessageID,
		AnswerCallback: callbackAnswer,
	}, nil
}
