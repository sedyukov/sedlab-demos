package main

import (
	"context"
	"net/url"
	"strconv"

	notify "github.com/sedyukov/sedlab-modules/modules/notify/go"
)

// Notifier is the slice of the sedlab "notify" module the app uses (*notify.Client implements it).
type Notifier interface {
	Send(ctx context.Context, in notify.SendInput) (*notify.Delivery, error)
}

// tell sends a bot message to a user and records it in the outbox (the emulator chat shows the outbox).
// Delivery goes through the notify module (channel "telegram"); Mini App buttons become an inline keyboard
// (notify.InlineKeyboard, web_app button). Idempotency keys stop a retried job from sending twice.
//
// Emulator users don't exist in Telegram, so in live mode only the outbox gets their messages.
func (a *App) tell(ctx context.Context, userID int64, kind, text string, btn *WebAppButton, idemKey string) {
	m := &Message{ChatID: userID, Text: text, Kind: kind}
	if btn != nil {
		m.Button = mustJSON(btn)
	}
	if err := a.store.AddMessage(m); err != nil {
		a.log.Error("outbox", "err", err)
	}
	platform := a.platformOf(userID)
	if isEmulatorUser(userID) && ((platform == "telegram" && a.cfg.TelegramLive) || (platform == "max" && a.cfg.MaxLive)) {
		return // emulator users don't exist in the live messenger
	}
	in := notify.SendInput{Channel: "telegram", To: strconv.FormatInt(userID, 10), HTML: text,
		IdempotencyKey: idemKey, Metadata: map[string]string{"kind": kind}}
	if platform == "max" {
		in.Channel, in.To = "max", "user:"+strconv.FormatInt(userID, 10)
	}
	if btn != nil {
		b := notify.Button{Text: btn.Text, WebApp: btn.URL}
		if platform == "max" { // open_app targets the bot; the screen goes in the payload → start_param
			b.WebApp, b.Payload = a.max.botName, screenOf(btn.URL)
		}
		rm, err := notify.InlineKeyboard([][]notify.Button{{b}})
		if err != nil {
			a.log.Warn("bad keyboard", "err", err)
		} else {
			in.ReplyMarkup = rm
		}
	}
	if _, err := a.notify.Send(ctx, in); err != nil {
		a.log.Warn("message not delivered", "kind", kind, "err", err)
	}
}

func screenOf(u string) string {
	if p, err := url.Parse(u); err == nil {
		return p.Query().Get("screen")
	}
	return ""
}
