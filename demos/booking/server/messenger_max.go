package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"time"

	mx "github.com/sedyukov/sedlab-modules/modules/max-core/go"
)

// maxMessenger — Max (VK) via max-core: same interface as telegram-core. Differences that matter here:
// no Stars (prepayment by card through the payments module), and the Mini App button is "open_app"
// pointing at the bot (the Mini App URL lives in the bot settings on the Max platform); the screen to open
// goes in the button payload → start_param of the launch data.
type maxMessenger struct {
	bot          *mx.Bot
	live         bool
	emulatorAuth bool
	botName      string  // MAX_BOT_USERNAME, target of open_app buttons
	emu          *mx.Bot // sandbox token: emulator launch data + sessions for emulator users
}

func newMaxMessenger(bot *mx.Bot, live, emulator bool, webhookSecret, botName string) *maxMessenger {
	return &maxMessenger{bot: bot, live: live, emulatorAuth: emulator, botName: botName,
		emu: mx.NewBot(mx.NewMockAPI(), mx.Config{Token: mx.MockToken, WebhookSecret: webhookSecret})}
}

func (m *maxMessenger) Platform() string { return "max" }

func (m *maxMessenger) AuthWebApp(initData string) (*WebUser, bool, error) {
	var auth *mx.WebAppAuth
	var err error
	emulated := false
	if m.live {
		auth, err = m.bot.ValidateInitData(mx.ValidateInitDataInput{InitData: initData})
	}
	if !m.live || (err != nil && m.emulatorAuth) {
		if !m.emulatorAuth {
			return nil, false, errors.New("emulator disabled")
		}
		auth, err = m.emu.ValidateInitData(mx.ValidateInitDataInput{InitData: initData})
		emulated = err == nil
	}
	if err != nil {
		return nil, false, err
	}
	if auth.User == nil || auth.User.ID == 0 {
		return nil, false, errors.New("init data has no user")
	}
	return &WebUser{ID: auth.User.ID, FirstName: auth.User.FirstName, Username: auth.User.Username, Lang: normLang(auth.User.LanguageCode)}, emulated, nil
}

func (m *maxMessenger) EmulatorInitData(u WebUser, authDate int64) (string, error) {
	r, err := m.emu.BuildInitData(mx.BuildInitDataInput{User: map[string]any{"id": u.ID, "first_name": u.FirstName, "username": u.Username, "language_code": u.Lang}, AuthDate: authDate})
	if err != nil {
		return "", err
	}
	return r.InitData, nil
}

func (m *maxMessenger) SendText(ctx context.Context, userID int64, text string, btn *WebAppButton) error {
	if m.live && isEmulatorUser(userID) {
		return nil
	}
	in := mx.SendMessageInput{UserID: userID, Text: text, ParseMode: "HTML", DisableLinkPreview: true}
	if btn != nil {
		payload := ""
		if u, err := url.Parse(btn.URL); err == nil {
			payload = u.Query().Get("screen") // e.g. "my" → start_param
		}
		rm, err := mx.InlineKeyboard([][]mx.Button{{{Text: btn.Text, WebApp: m.botName, Payload: payload}}})
		if err != nil {
			return err
		}
		in.ReplyMarkup = rm
	}
	bot := m.bot
	if isEmulatorUser(userID) {
		bot = m.emu
	}
	_, err := bot.SendMessage(ctx, in)
	return err
}

func (m *maxMessenger) StarsInvoiceLink(context.Context, StarsInvoice, bool) (string, error) {
	return "", ErrUnsupported
}
func (m *maxMessenger) AnswerPreCheckout(context.Context, string, bool, string) error {
	return ErrUnsupported
}
func (m *maxMessenger) RefundStars(context.Context, int64, string) error { return ErrUnsupported }

func (m *maxMessenger) ParseUpdate(ctx context.Context, headers map[string]string, body []byte) (*Update, error) {
	u, err := m.bot.ParseUpdate(ctx, mx.ParseUpdateInput{Headers: headers, Body: string(body)})
	if err != nil {
		return nil, err
	}
	return &Update{Kind: u.Kind, ChatID: u.ChatID, UserID: u.UserID, Text: u.Text, Command: u.Command, Duplicate: u.Duplicate}, nil
}

func (m *maxMessenger) SetWebhook(ctx context.Context, u string) error {
	_, err := m.bot.SetWebhook(ctx, mx.WebhookInput{URL: u, AllowedUpdates: []string{"message_created", "message_callback", "bot_started"}})
	return err
}

func (m *maxMessenger) IssueSession(u *WebUser, ttl time.Duration) (string, int64, error) {
	bot := m.bot
	if isEmulatorUser(u.ID) {
		bot = m.emu
	}
	t, err := bot.IssueSession(mx.IssueSessionInput{User: mx.WebAppUser{ID: u.ID, FirstName: u.FirstName, Username: u.Username}, TTLSec: int64(ttl.Seconds())})
	if err != nil {
		return "", 0, err
	}
	return t.Token, t.ExpiresAt, nil
}

func (m *maxMessenger) VerifySession(token string) (int64, error) {
	s, err := m.bot.VerifySession(mx.VerifySessionInput{Token: token})
	if err != nil && m.emulatorAuth {
		s, err = m.emu.VerifySession(mx.VerifySessionInput{Token: token})
	}
	if err != nil {
		return 0, err
	}
	return s.UserID, nil
}

// emulatorUpdate builds a Max "message_created" webhook body for text typed in the emulator.
func maxEmulatorUpdate(userID int64, mid, text string, ts int64) []byte {
	b, _ := json.Marshal(map[string]any{
		"update_type": "message_created", "timestamp": ts * 1000,
		"message": map[string]any{
			"recipient": map[string]any{"chat_id": userID, "user_id": 0},
			"sender":    map[string]any{"user_id": userID},
			"timestamp": ts * 1000,
			"body":      map[string]any{"mid": mid, "text": text},
		},
	})
	return b
}
