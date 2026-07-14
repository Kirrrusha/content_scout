package bot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const asyncLLMOperationTimeout = 10 * time.Minute

type Service struct {
	api    *tgbotapi.BotAPI
	router *Router
	logger *slog.Logger
}

func NewService(token string, ownerID int64, logger *slog.Logger) (*Service, error) {
	return NewServiceWithAuth(token, ownerID, nil, logger)
}

func NewServiceWithAuth(token string, ownerID int64, auth AuthController, logger *slog.Logger) (*Service, error) {
	return NewServiceWithControllers(token, ownerID, auth, nil, logger)
}

func NewServiceWithControllers(token string, ownerID int64, auth AuthController, sync SyncController, logger *slog.Logger) (*Service, error) {
	return NewServiceWithAllControllers(token, ownerID, auth, sync, nil, logger)
}

func NewServiceWithAllControllers(token string, ownerID int64, auth AuthController, sync SyncController, groups GroupController, logger *slog.Logger) (*Service, error) {
	return NewServiceWithRuntime(token, ownerID, auth, sync, groups, nil, logger)
}

func NewServiceWithRuntime(token string, ownerID int64, auth AuthController, sync SyncController, groups GroupController, collector CollectionController, logger *slog.Logger) (*Service, error) {
	return NewServiceWithServices(token, ownerID, auth, sync, groups, collector, nil, logger)
}

func NewServiceWithServices(token string, ownerID int64, auth AuthController, sync SyncController, groups GroupController, collector CollectionController, summary SummaryController, logger *slog.Logger) (*Service, error) {
	return NewServiceWithBrowser(token, ownerID, auth, sync, groups, collector, summary, nil, logger)
}

func NewServiceWithBrowser(token string, ownerID int64, auth AuthController, sync SyncController, groups GroupController, collector CollectionController, summary SummaryController, browser SummaryBrowserController, logger *slog.Logger) (*Service, error) {
	return NewServiceWithArticle(token, ownerID, auth, sync, groups, collector, summary, browser, nil, logger)
}

func NewServiceWithArticle(token string, ownerID int64, auth AuthController, sync SyncController, groups GroupController, collector CollectionController, summary SummaryController, browser SummaryBrowserController, articles ArticleController, logger *slog.Logger) (*Service, error) {
	return NewServiceWithExports(token, ownerID, auth, sync, groups, collector, summary, browser, articles, nil, logger)
}

func NewServiceWithExports(token string, ownerID int64, auth AuthController, sync SyncController, groups GroupController, collector CollectionController, summary SummaryController, browser SummaryBrowserController, articles ArticleController, exports ExportController, logger *slog.Logger) (*Service, error) {
	if token == "" {
		return nil, fmt.Errorf("telegram bot token is not configured")
	}
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("create telegram bot api: %w", err)
	}
	return &Service{
		api:    api,
		router: NewRouterWithExports(ownerID, NewMemoryStateStore(), auth, sync, groups, collector, summary, browser, articles, exports),
		logger: logger,
	}, nil
}

func (s *Service) SetSchedules(controller ScheduleController) {
	s.router.SetSchedules(controller)
}

func (s *Service) Run(ctx context.Context) error {
	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 30

	updates := s.api.GetUpdatesChan(updateConfig)
	s.logger.Info("telegram bot polling started")

	for {
		select {
		case <-ctx.Done():
			s.api.StopReceivingUpdates()
			return nil
		case update := <-updates:
			if err := s.handleUpdate(ctx, update); err != nil {
				s.logger.Warn("telegram update handling failed", "error", err)
			}
		}
	}
}

func (s *Service) handleUpdate(ctx context.Context, update tgbotapi.Update) error {
	in, ok := incomingFromUpdate(update)
	if !ok {
		return nil
	}
	s.logIncoming(in)
	answeredCallback := false
	if in.CallbackID != "" {
		callback := tgbotapi.NewCallback(in.CallbackID, "Выполняю...")
		if _, err := s.api.Request(callback); err != nil {
			s.logger.Debug("telegram callback pre-answer failed", "error", err)
		} else {
			answeredCallback = true
		}
	}

	if isAsyncArticleConversionCallback(in) {
		progress := Outgoing{
			ChatID:        in.ChatID,
			Text:          "Создаю черновик статьи.\n\nЭто может занять минуту: модель разворачивает тему в полноценный текст.",
			Menu:          articleProgressMenu(),
			EditMessageID: in.CallbackMessage,
		}
		if !answeredCallback {
			progress.CallbackID = in.CallbackID
			progress.AnswerCallback = "Создаю статью."
		}
		if err := s.Send(ctx, progress); err != nil {
			return err
		}
		s.runAsyncArticleConversion(ctx, in)
		return nil
	}

	if isAsyncSummaryGenerationCallback(in) {
		progress := Outgoing{
			ChatID:        in.ChatID,
			Text:          "Создаю сводку.\n\nЭто может занять минуту: модель группирует сообщения по темам и проверяет источники.",
			Menu:          summaryProgressMenu(),
			EditMessageID: in.CallbackMessage,
		}
		if !answeredCallback {
			progress.CallbackID = in.CallbackID
			progress.AnswerCallback = "Создаю сводку."
		}
		if err := s.Send(ctx, progress); err != nil {
			return err
		}
		s.runAsyncSummaryGeneration(ctx, in)
		return nil
	}

	out, err := s.router.Handle(ctx, in)
	if err != nil {
		s.logger.Warn("telegram route failed", "kind", in.Kind, "user_id", in.UserID, "chat_id", in.ChatID, "error", err)
		return err
	}
	if answeredCallback {
		out.CallbackID = ""
	}
	return s.Send(ctx, out)
}

func (s *Service) runAsyncArticleConversion(parent context.Context, in Incoming) {
	s.logger.Info("telegram async article conversion started",
		"user_id", in.UserID,
		"chat_id", in.ChatID,
		"message_id", in.CallbackMessage,
		"callback_data", in.CallbackData,
	)
	go func() {
		ctx, cancel := context.WithTimeout(parent, asyncLLMOperationTimeout)
		defer cancel()

		background := in
		background.CallbackID = ""
		out, err := s.router.Handle(ctx, background)
		if err != nil {
			s.logger.Warn("telegram async article conversion failed",
				"user_id", in.UserID,
				"chat_id", in.ChatID,
				"message_id", in.CallbackMessage,
				"callback_data", in.CallbackData,
				"error", err,
			)
			out = Outgoing{
				ChatID:        in.ChatID,
				Text:          "Не удалось создать статью.\n\nТехническая причина: " + err.Error(),
				Menu:          articleProgressMenu(),
				EditMessageID: in.CallbackMessage,
			}
		}
		out.CallbackID = ""
		if out.ChatID == 0 {
			out.ChatID = in.ChatID
		}
		if out.EditMessageID == 0 {
			out.EditMessageID = in.CallbackMessage
		}
		if sendErr := s.Send(ctx, out); sendErr != nil {
			s.logger.Warn("telegram async article conversion result send failed",
				"user_id", in.UserID,
				"chat_id", in.ChatID,
				"message_id", in.CallbackMessage,
				"callback_data", in.CallbackData,
				"error", sendErr,
			)
			return
		}
		s.logger.Info("telegram async article conversion finished",
			"user_id", in.UserID,
			"chat_id", in.ChatID,
			"message_id", in.CallbackMessage,
			"callback_data", in.CallbackData,
		)
	}()
}

func (s *Service) runAsyncSummaryGeneration(parent context.Context, in Incoming) {
	s.logger.Info("telegram async summary generation started",
		"user_id", in.UserID,
		"chat_id", in.ChatID,
		"message_id", in.CallbackMessage,
		"callback_data", in.CallbackData,
	)
	go func() {
		ctx, cancel := context.WithTimeout(parent, asyncLLMOperationTimeout)
		defer cancel()

		background := in
		background.CallbackID = ""
		out, err := s.router.Handle(ctx, background)
		if err != nil {
			s.logger.Warn("telegram async summary generation failed",
				"user_id", in.UserID,
				"chat_id", in.ChatID,
				"message_id", in.CallbackMessage,
				"callback_data", in.CallbackData,
				"error", err,
			)
			out = Outgoing{
				ChatID:        in.ChatID,
				Text:          "Не удалось создать сводку.\n\nТехническая причина: " + err.Error(),
				Menu:          summaryProgressMenu(),
				EditMessageID: in.CallbackMessage,
			}
		}
		out.CallbackID = ""
		if out.ChatID == 0 {
			out.ChatID = in.ChatID
		}
		if out.EditMessageID == 0 {
			out.EditMessageID = in.CallbackMessage
		}
		if sendErr := s.Send(ctx, out); sendErr != nil {
			s.logger.Warn("telegram async summary generation result send failed",
				"user_id", in.UserID,
				"chat_id", in.ChatID,
				"message_id", in.CallbackMessage,
				"callback_data", in.CallbackData,
				"error", sendErr,
			)
			return
		}
		s.logger.Info("telegram async summary generation finished",
			"user_id", in.UserID,
			"chat_id", in.ChatID,
			"message_id", in.CallbackMessage,
			"callback_data", in.CallbackData,
		)
	}()
}

func isAsyncArticleConversionCallback(in Incoming) bool {
	if in.Kind != IncomingCallback {
		return false
	}
	return strings.HasPrefix(in.CallbackData, "art:summary:") || strings.HasPrefix(in.CallbackData, "art:topic:")
}

func isAsyncSummaryGenerationCallback(in Incoming) bool {
	if in.Kind != IncomingCallback {
		return false
	}
	return strings.HasPrefix(in.CallbackData, "newsum:generate:")
}

func articleProgressMenu() Menu {
	return Menu{
		{{Text: "К статьям", Data: "art:list"}, {Text: "Назад", Data: ActionBackHome}},
	}
}

func summaryProgressMenu() Menu {
	return Menu{
		{{Text: "История", Data: "sum:list"}, {Text: "Назад", Data: ActionBackHome}},
	}
}

func (s *Service) Send(_ context.Context, out Outgoing) error {
	out.Text = telegramText(out.Text)
	s.logOutgoing(out)
	if out.CallbackID != "" {
		callback := tgbotapi.NewCallback(out.CallbackID, out.AnswerCallback)
		_, _ = s.api.Request(callback)
	}

	if out.DocumentPath != "" {
		doc := tgbotapi.NewDocument(out.ChatID, tgbotapi.FilePath(out.DocumentPath))
		doc.Caption = out.Text
		doc.ReplyMarkup = telegramMenu(out.Menu)
		if _, err := s.api.Send(doc); err != nil {
			s.logger.Warn("telegram send document failed", "chat_id", out.ChatID, "document_path", out.DocumentPath, "error", err)
			return fmt.Errorf("send telegram document: %w", err)
		}
		s.logger.Info("telegram document sent", "chat_id", out.ChatID, "document_path", out.DocumentPath)
		return nil
	}

	if out.EditMessageID != 0 {
		edit := tgbotapi.NewEditMessageText(out.ChatID, out.EditMessageID, out.Text)
		edit.ReplyMarkup = telegramMenu(out.Menu)
		if _, err := s.api.Send(edit); err != nil {
			s.logger.Warn("telegram edit failed; sending fallback message", "chat_id", out.ChatID, "message_id", out.EditMessageID, "error", err)
			msg := tgbotapi.NewMessage(out.ChatID, out.Text)
			msg.ReplyMarkup = telegramMenu(out.Menu)
			if _, sendErr := s.api.Send(msg); sendErr != nil {
				s.logger.Warn("telegram fallback message failed", "chat_id", out.ChatID, "message_id", out.EditMessageID, "error", sendErr)
				return fmt.Errorf("edit telegram message: %w; fallback send telegram message: %w", err, sendErr)
			}
			s.logger.Info("telegram fallback message sent", "chat_id", out.ChatID, "original_message_id", out.EditMessageID)
			return nil
		}
		s.logger.Info("telegram message edited", "chat_id", out.ChatID, "message_id", out.EditMessageID)
		return nil
	}

	msg := tgbotapi.NewMessage(out.ChatID, out.Text)
	msg.ReplyMarkup = telegramMenu(out.Menu)
	if _, err := s.api.Send(msg); err != nil {
		s.logger.Warn("telegram send message failed", "chat_id", out.ChatID, "error", err)
		return fmt.Errorf("send telegram message: %w", err)
	}
	s.logger.Info("telegram message sent", "chat_id", out.ChatID)
	return nil
}

func (s *Service) logIncoming(in Incoming) {
	if in.Kind == IncomingCallback {
		s.logger.Info("telegram callback received",
			"user_id", in.UserID,
			"chat_id", in.ChatID,
			"message_id", in.CallbackMessage,
			"callback_data", in.CallbackData,
		)
		return
	}
	command := in.Command
	if command == "" {
		command = commandFromText(in.Text)
	}
	s.logger.Info("telegram message received",
		"user_id", in.UserID,
		"chat_id", in.ChatID,
		"command", command,
		"text", safeTelegramTextForLog(in.Text),
	)
}

func (s *Service) logOutgoing(out Outgoing) {
	action := "send_message"
	if out.DocumentPath != "" {
		action = "send_document"
	} else if out.EditMessageID != 0 {
		action = "edit_message"
	}
	s.logger.Info("telegram outgoing prepared",
		"chat_id", out.ChatID,
		"action", action,
		"edit_message_id", out.EditMessageID,
		"callback_answer", out.AnswerCallback,
		"menu_rows", len(out.Menu),
		"text_len", len([]rune(out.Text)),
		"text_preview", previewText(out.Text, 120),
	)
}

func telegramText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "Готово."
	}
	const limit = 3900
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "\n\nТекст сокращен. Полную версию можно открыть по темам или экспортировать."
}

func commandFromText(text string) string {
	fields := strings.Fields(text)
	if len(fields) == 0 || !strings.HasPrefix(fields[0], "/") {
		return ""
	}
	return strings.TrimPrefix(fields[0], "/")
}

func safeTelegramTextForLog(text string) string {
	command := strings.ToLower(commandFromText(text))
	switch command {
	case "phone", "code", "password":
		return "<redacted>"
	default:
		return previewText(text, 240)
	}
}

func previewText(text string, limit int) string {
	text = strings.TrimSpace(strings.Join(strings.Fields(text), " "))
	if limit <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit-1]) + "…"
}

func incomingFromUpdate(update tgbotapi.Update) (Incoming, bool) {
	if update.Message != nil {
		from := update.Message.From
		if from == nil {
			return Incoming{}, false
		}
		return Incoming{
			Kind:    IncomingMessage,
			UserID:  from.ID,
			ChatID:  update.Message.Chat.ID,
			Text:    update.Message.Text,
			Command: update.Message.Command(),
		}, true
	}
	if update.CallbackQuery != nil {
		from := update.CallbackQuery.From
		message := update.CallbackQuery.Message
		if from == nil || message == nil {
			return Incoming{}, false
		}
		return Incoming{
			Kind:            IncomingCallback,
			UserID:          from.ID,
			ChatID:          message.Chat.ID,
			MessageID:       update.CallbackQuery.Message.MessageID,
			CallbackID:      update.CallbackQuery.ID,
			CallbackData:    update.CallbackQuery.Data,
			CallbackMessage: update.CallbackQuery.Message.MessageID,
		}, true
	}
	return Incoming{}, false
}

func telegramMenu(menu Menu) *tgbotapi.InlineKeyboardMarkup {
	if len(menu) == 0 {
		return nil
	}
	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(menu))
	for _, row := range menu {
		buttons := make([]tgbotapi.InlineKeyboardButton, 0, len(row))
		for _, button := range row {
			buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData(button.Text, button.Data))
		}
		rows = append(rows, buttons)
	}
	markup := tgbotapi.NewInlineKeyboardMarkup(rows...)
	return &markup
}

func RunWithShutdown(ctx context.Context, service *Service) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- service.Run(ctx)
	}()

	select {
	case <-ctx.Done():
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case err := <-errCh:
			return err
		case <-timer.C:
			return nil
		}
	case err := <-errCh:
		return err
	}
}
