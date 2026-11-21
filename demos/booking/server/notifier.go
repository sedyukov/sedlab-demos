package main

import (
	"context"
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
	if a.cfg.TelegramLive && isEmulatorUser(userID) {
		return
	}
	in := notify.SendInput{Channel: "telegram", To: strconv.FormatInt(userID, 10), HTML: text,
		IdempotencyKey: idemKey, Metadata: map[string]string{"kind": kind}}
	if btn != nil {
		rm, err := notify.InlineKeyboard([][]notify.Button{{{Text: btn.Text, WebApp: btn.URL}}})
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
