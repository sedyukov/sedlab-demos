package main

// Max login bot (on the RU host, direct to platform-api2.max.ru) + lead notifications to the owner via notify/max.

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	maxcore "github.com/sedyukov/sedlab-modules/modules/max-core/go"
	notify "github.com/sedyukov/sedlab-modules/modules/notify/go"
)

// maxRawUser: name fields are not in the normalised Update; read them from the raw body.
type maxRawUser struct {
	UserID    int64  `json:"user_id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Name      string `json:"name"`
	Username  string `json:"username"`
}

func maxUserFromRaw(raw []byte) maxRawUser {
	var b struct {
		User     *maxRawUser `json:"user"`
		Callback *struct {
			User *maxRawUser `json:"user"`
		} `json:"callback"`
		Message *struct {
			Sender *maxRawUser `json:"sender"`
		} `json:"message"`
	}
	_ = json.Unmarshal(raw, &b)
	switch {
	case b.Callback != nil && b.Callback.User != nil:
		return *b.Callback.User
	case b.User != nil:
		return *b.User
	case b.Message != nil && b.Message.Sender != nil:
		return *b.Message.Sender
	}
	return maxRawUser{}
}

func (u maxRawUser) display() string {
	if n := strings.TrimSpace(u.FirstName + " " + u.LastName); n != "" {
		return n
	}
	return u.Name
}

func (a *App) maxWebhook(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	hdr := map[string]string{}
	for k := range r.Header {
		hdr[k] = r.Header.Get(k)
	}
	u, err := a.max.ParseUpdate(r.Context(), maxcore.ParseUpdateInput{Headers: hdr, Body: string(body)})
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.WriteHeader(200) // ack fast; Max retries otherwise
	if u.Duplicate || !a.providers.Enabled("max") {
		return
	}
	ctx := context.WithoutCancel(r.Context())
	from := maxUserFromRaw(body)
	switch {
	case u.Command == "start" && strings.HasPrefix(u.CommandArgs, "login_"):
		nonce := strings.TrimPrefix(u.CommandArgs, "login_")
		host := strings.TrimPrefix(strings.TrimPrefix(a.cfg.PublicURL, "https://"), "http://")
		name := from.display()
		var text, btn string
		if strings.HasPrefix(strings.ToLower(u.UserLocale), "ru") || u.UserLocale == "" { // legal/ui-texts.md §2
			text = "Войти на " + host + " как " + name + "? Если вход начали не вы — просто проигнорируйте это сообщение."
			btn = "Подтвердить вход"
		} else {
			text = "Sign in to " + host + " as " + name + "? If you didn't start this, just ignore this message."
			btn = "Confirm sign-in"
		}
		kb, _ := maxcore.InlineKeyboard([][]maxcore.Button{{{Text: btn, Data: "login:" + nonce}}})
		a.maxSend(ctx, u, text, kb)
	case u.Kind == "callback_query" && strings.HasPrefix(u.Data, "login:"):
		uid := from.UserID
		if uid == 0 {
			uid = u.UserID
		}
		ok, err := a.store.ConfirmNonce(ctx, strings.TrimPrefix(u.Data, "login:"), "max", strconv.FormatInt(uid, 10), from.display(), from.Username, a.now())
		text := "Готово — вернитесь в браузер. / Done — return to your browser."
		if err != nil || !ok {
			text = "Ссылка устарела — начните вход заново. / Link expired — start again."
		}
		_, _ = a.max.AnswerCallbackQuery(ctx, maxcore.AnswerCallbackInput{CallbackQueryID: u.CallbackQueryID, Text: text})
	case u.Command == "start" || u.Command == "id":
		a.maxSend(ctx, u, "Бот входа sedlab demo. Войти: "+a.cfg.PublicURL+"/login\nВаш Max user_id: "+strconv.FormatInt(u.UserID, 10), nil)
	}
}

func (a *App) maxSend(ctx context.Context, u *maxcore.Update, text string, kb json.RawMessage) {
	in := maxcore.SendMessageInput{ChatID: u.ChatID, Text: text, ReplyMarkup: kb}
	if in.ChatID == 0 {
		in.UserID = u.UserID
	}
	if _, err := a.max.SendMessage(ctx, in); err != nil {
		slog.Warn("max send", "err", err)
	}
}

// notifyOwner: Max dialog with the owner (AUTH_OWNER_MAX_USER_ID). No personal data in the text — only lead id + link.
func (a *App) notifyOwner(ctx context.Context, text, idem string) bool {
	if a.cfg.OwnerMaxUser == "" {
		return false
	}
	_, err := a.notify.Send(ctx, notify.SendInput{Channel: "max", To: "user:" + a.cfg.OwnerMaxUser, Text: text, IdempotencyKey: idem})
	if err != nil {
		slog.Warn("notify owner", "err", err)
		return false
	}
	return true
}

func (a *App) notifyLeads(ctx context.Context) {
	if a.cfg.OwnerMaxUser == "" {
		return
	}
	leads, err := a.store.UnnotifiedLeads(ctx)
	if err != nil {
		return
	}
	for _, l := range leads {
		demo := l.FirstDemo
		if demo == "" {
			demo = "—"
		}
		text := "🆕 Новый аккаунт sedlab: " + l.Provider + ", демо: " + demo + "\n" + a.cfg.PublicURL + "/admin/leads/" + l.ID
		if a.notifyOwner(ctx, text, "lead-"+l.ID) {
			_ = a.store.MarkLeadNotified(ctx, l.ID, a.now())
		}
	}
}
