package bot

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

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

	out, err := s.router.Handle(ctx, in)
	if err != nil {
		return err
	}
	return s.Send(ctx, out)
}

func (s *Service) Send(_ context.Context, out Outgoing) error {
	if out.CallbackID != "" {
		callback := tgbotapi.NewCallback(out.CallbackID, out.AnswerCallback)
		_, _ = s.api.Request(callback)
	}

	if out.DocumentPath != "" {
		doc := tgbotapi.NewDocument(out.ChatID, tgbotapi.FilePath(out.DocumentPath))
		doc.Caption = out.Text
		doc.ReplyMarkup = telegramMenu(out.Menu)
		if _, err := s.api.Send(doc); err != nil {
			return fmt.Errorf("send telegram document: %w", err)
		}
		return nil
	}

	if out.EditMessageID != 0 {
		edit := tgbotapi.NewEditMessageText(out.ChatID, out.EditMessageID, out.Text)
		edit.ReplyMarkup = telegramMenu(out.Menu)
		if _, err := s.api.Send(edit); err != nil {
			return fmt.Errorf("edit telegram message: %w", err)
		}
		return nil
	}

	msg := tgbotapi.NewMessage(out.ChatID, out.Text)
	msg.ReplyMarkup = telegramMenu(out.Menu)
	if _, err := s.api.Send(msg); err != nil {
		return fmt.Errorf("send telegram message: %w", err)
	}
	return nil
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
