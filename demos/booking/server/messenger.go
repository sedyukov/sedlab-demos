package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	tg "github.com/sedyukov/sedlab-modules/modules/telegram-core/go"
)

// Messenger is the platform seam: Telegram (telegram-core) and Max (max-core, same interface).
// Booking logic only talks to this interface.
type Messenger interface {
	Platform() string
	// AuthWebApp validates Mini App launch data and returns the user (emulated=true if it was signed
	// by the built-in browser emulator rather than the real platform).
	AuthWebApp(initData string) (u *WebUser, emulated bool, err error)
	SendText(ctx context.Context, chatID int64, text string, btn *WebAppButton) error
	// Stars (Telegram-only digital currency). Other platforms return ErrUnsupported.
	StarsInvoiceLink(ctx context.Context, in StarsInvoice, emulated bool) (string, error)
	AnswerPreCheckout(ctx context.Context, queryID string, ok bool, errMsg string) error
	RefundStars(ctx context.Context, userID int64, chargeID string) error
	ParseUpdate(ctx context.Context, headers map[string]string, body []byte) (*Update, error)
	SetWebhook(ctx context.Context, url string) error
	AnswerCallback(ctx context.Context, callbackID, text string) error
	// Sessions for the Mini App API after launch-data validation.
	IssueSession(u *WebUser, ttl time.Duration) (token string, exp int64, err error)
	VerifySession(token string) (userID int64, err error)
	// EmulatorInitData signs launch data for the browser emulator with the module's sandbox token.
	EmulatorInitData(u WebUser, authDate int64) (string, error)
}

// Update is a platform-neutral incoming update (telegram-core and max-core already share the field names).
type Update struct {
	Kind               string // message | command | callback_query | pre_checkout_query | successful_payment | other
	ChatID, UserID     int64
	Text, Command      string
	Data               string // callback data
	CallbackQueryID    string
	PreCheckoutQueryID string
	Payment            *tg.Payment // Telegram Stars only
	Duplicate          bool
}

var ErrUnsupported = errors.New("not supported on this platform")

type WebUser struct {
	ID        int64
	FirstName string
	Username  string
	Lang      string
}

// WebAppButton is one inline button: a Mini App button (URL) or a callback button (Data).
type WebAppButton struct {
	Text string `json:"text"`
	URL  string `json:"url,omitempty"`
	Data string `json:"data,omitempty"`
}

type StarsInvoice struct {
	Title, Description, Payload string
	Amount                      int64 // XTR
}

// ---- Telegram (telegram-core module) ----

type telegramMessenger struct {
	bot          *tg.Bot
	live         bool    // real Bot API (not the mock transport)
	emulatorAuth bool    // also accept initData signed by the emulator (mock token)
	emu          *tg.Bot // signs/validates emulator initData with the public sandbox token
}

// emu: a bot on the mock transport with the public sandbox token and the same webhook secret — it signs
// emulator initData and produces simulated Stars payment updates (telegram-core MockStarsPayment).
func newTelegramMessenger(bot *tg.Bot, live, emulator bool, webhookSecret string) *telegramMessenger {
	m := &telegramMessenger{bot: bot, live: live, emulatorAuth: emulator}
	m.emu = tg.NewBot(tg.NewMockAPI(), tg.Config{Token: tg.MockToken, WebhookSecret: webhookSecret})
	return m
}

func (m *telegramMessenger) Platform() string { return "telegram" }

func (m *telegramMessenger) AuthWebApp(initData string) (*WebUser, bool, error) {
	var auth *tg.WebAppAuth
	var err error
	emulated := false
	if m.live {
		auth, err = m.bot.ValidateInitData(tg.ValidateInitDataInput{InitData: initData})
	}
	if !m.live || (err != nil && m.emulatorAuth) {
		if !m.live && !m.emulatorAuth {
			return nil, false, errors.New("emulator disabled")
		}
		auth, err = m.emu.ValidateInitData(tg.ValidateInitDataInput{InitData: initData})
		emulated = err == nil
	}
	if err != nil {
		return nil, false, err
	}
	if auth.User == nil || auth.User.ID == 0 {
		return nil, false, errors.New("initData has no user")
	}
	return &WebUser{ID: auth.User.ID, FirstName: auth.User.FirstName, Username: auth.User.Username, Lang: normLang(auth.User.LanguageCode)}, emulated, nil
}

// EmulatorInitData signs initData for the in-browser emulator (telegram-core BuildInitData, sandbox token).
func (m *telegramMessenger) EmulatorInitData(u WebUser, authDate int64) (string, error) {
	user := map[string]any{"id": u.ID, "first_name": u.FirstName, "username": u.Username, "language_code": u.Lang}
	r, err := m.emu.BuildInitData(tg.BuildInitDataInput{User: user, AuthDate: authDate})
	if err != nil {
		return "", err
	}
	return r.InitData, nil
}

func (m *telegramMessenger) SendText(ctx context.Context, chatID int64, text string, btn *WebAppButton) error {
	if m.live && isEmulatorUser(chatID) {
		return nil // emulator users don't exist in Telegram; their chat lives in the outbox only
	}
	in := tg.SendMessageInput{ChatID: chatID, Text: text, ParseMode: "HTML", DisableLinkPreview: true}
	if btn != nil {
		in.ReplyMarkup = json.RawMessage(mustJSON(map[string]any{"inline_keyboard": [][]map[string]any{{{"text": btn.Text, "web_app": map[string]string{"url": btn.URL}}}}}))
	}
	_, err := m.bot.SendMessage(ctx, in)
	return err
}

func (m *telegramMessenger) StarsInvoiceLink(ctx context.Context, in StarsInvoice, emulated bool) (string, error) {
	bot := m.bot
	if emulated {
		bot = m.emu // the emulator renders its own Stars sheet for this link
	}
	l, err := bot.CreateInvoiceLink(ctx, tg.InvoiceLinkInput{Title: in.Title, Description: in.Description, Payload: in.Payload,
		Currency: "XTR", Prices: []tg.LabeledPrice{{Label: in.Title, Amount: in.Amount}}})
	if err != nil {
		return "", err
	}
	return l.URL, nil
}

func (m *telegramMessenger) AnswerPreCheckout(ctx context.Context, id string, ok bool, errMsg string) error {
	_, err := m.bot.AnswerPreCheckoutQuery(ctx, tg.PreCheckoutAnswer{PreCheckoutQueryID: id, OK: ok, ErrorMessage: errMsg})
	return err
}

func (m *telegramMessenger) RefundStars(ctx context.Context, userID int64, chargeID string) error {
	_, err := m.bot.RefundStarPayment(ctx, tg.StarRefundInput{UserID: userID, TelegramPaymentChargeID: chargeID})
	return err
}

func (m *telegramMessenger) ParseUpdate(ctx context.Context, headers map[string]string, body []byte) (*Update, error) {
	u, err := m.bot.ParseUpdate(ctx, tg.ParseUpdateInput{Headers: headers, Body: string(body)})
	if err != nil {
		return nil, err
	}
	return &Update{Kind: u.Kind, ChatID: u.ChatID, UserID: u.UserID, Text: u.Text, Command: u.Command,
		Data: u.Data, CallbackQueryID: u.CallbackQueryID,
		PreCheckoutQueryID: u.PreCheckoutQueryID, Payment: u.Payment, Duplicate: u.Duplicate}, nil
}

func (m *telegramMessenger) IssueSession(u *WebUser, ttl time.Duration) (string, int64, error) {
	t, err := m.bot.IssueSession(tg.IssueSessionInput{User: tg.WebAppUser{ID: u.ID, FirstName: u.FirstName, Username: u.Username}, TTLSec: int64(ttl.Seconds())})
	if err != nil {
		return "", 0, err
	}
	return t.Token, t.ExpiresAt, nil
}

func (m *telegramMessenger) VerifySession(token string) (int64, error) {
	s, err := m.bot.VerifySession(tg.VerifySessionInput{Token: token})
	if err != nil {
		return 0, err
	}
	return s.UserID, nil
}

// EmulatorStarsPayment simulates what Telegram sends after the user pays an invoice:
// ready webhook requests (pre_checkout_query, successful_payment) with the secret header.
func (m *telegramMessenger) EmulatorStarsPayment(userID int64, payload string, amount int64) (*tg.MockStarsPayment, error) {
	return m.emu.MockStarsPayment(tg.MockStarsPaymentInput{UserID: userID, Payload: payload, Amount: amount})
}

func (m *telegramMessenger) AnswerCallback(ctx context.Context, id, text string) error {
	if id == "" {
		return nil
	}
	_, err := m.bot.AnswerCallbackQuery(ctx, tg.AnswerCallbackInput{CallbackQueryID: id, Text: text})
	return err
}

func (m *telegramMessenger) SetWebhook(ctx context.Context, url string) error {
	_, err := m.bot.SetWebhook(ctx, tg.WebhookInput{URL: url, AllowedUpdates: []string{"message", "callback_query", "pre_checkout_query"}})
	return err
}

// ---- helpers ----

// Emulator users get ids ≥ EmulatorUserBase (above any real Telegram/Max id, still JS-safe), so they never
// collide with real users and are recognisable without extra claims in the session token.
// Telegram guests: base+1e6…; Max guests: base+MaxEmulatorOffset….
const (
	EmulatorUserBase  int64 = 4_000_000_000_000_000
	MaxEmulatorOffset int64 = 2_000_000_000_000
)

func isEmulatorUser(id int64) bool { return id >= EmulatorUserBase }

func normLang(l string) string {
	if strings.HasPrefix(strings.ToLower(l), "ru") {
		return "ru"
	}
	return "en"
}

func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
