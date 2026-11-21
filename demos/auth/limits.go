package main

// Limits: accounts module format (accounts.LimitsConfig; per_day_global / monthly_budget <= 0 = no cap,
// per_user_day <= 0 or disabled = off). Provider labels with country (legal/ui-texts.md §4) live here.

import (
	_ "embed"
	"os"
	"time"

	accounts "github.com/sedyukov/sedlab-modules/modules/accounts/go"
)

//go:embed limits.json
var defaultLimits []byte

type Limits struct {
	*accounts.LimitsConfig
}

// LoadLimits: embedded limits.json, or AUTH_LIMITS_FILE when set.
func LoadLimits(path string) (*Limits, error) {
	b := defaultLimits
	if path != "" {
		var err error
		if b, err = os.ReadFile(path); err != nil {
			return nil, err
		}
	}
	c, err := accounts.ParseLimits(b)
	if err != nil {
		return nil, err
	}
	return &Limits{c}, nil
}

func (l *Limits) For(demo, resource string) (accounts.ResourceLimit, bool) {
	return l.Resolve(demo, resource)
}

func nextDay(now int64) int64 {
	t := time.Unix(now, 0).UTC()
	return time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, time.UTC).Unix()
}

var providerLabels = map[string][2]string{
	"openai": {"OpenAI (США)", "OpenAI (USA)"}, "anthropic": {"Anthropic (США)", "Anthropic (USA)"},
	"gigachat": {"GigaChat (Россия)", "GigaChat (Russia)"}, "yandex-llm": {"YandexGPT (Россия)", "YandexGPT (Russia)"},
	"yandex-stt": {"Yandex SpeechKit (Россия)", "Yandex SpeechKit (Russia)"}, "yandex-tts": {"Yandex SpeechKit TTS (Россия)", "Yandex SpeechKit TTS (Russia)"},
	"yookassa-test": {"ЮKassa, тест (Россия)", "YooKassa test (Russia)"}, "stripe-test": {"Stripe, тест (США)", "Stripe test (USA)"},
	"amocrm": {"amoCRM (Россия)", "amoCRM (Russia)"}, "hubspot-dev": {"HubSpot, dev (США)", "HubSpot dev (USA)"},
	"telegram": {"Telegram (ОАЭ)", "Telegram (UAE)"}, "max": {"Max (Россия)", "Max (Russia)"},
}

func providerLabel2(res, lang string) string {
	l, ok := providerLabels[res]
	if !ok {
		return res
	}
	if lang == "en" {
		return l[1]
	}
	return l[0]
}
