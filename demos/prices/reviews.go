package main

// Reviews: buyer reviews from the seller account (marketplaces module) plus reviews the visitor pastes in.
// legal-ru (prices-v1, terms 3a): buyer names are personal data of third parties, so they are dropped at load and
// the texts are de-identified (ai.MaskPII) before they are shown or sent to a model; pasting own texts needs the
// prices-v1 consent, its version and time are kept with the session.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	ai "github.com/sedyukov/sedlab-modules/modules/ai/go"
	marketplaces "github.com/sedyukov/sedlab-modules/modules/marketplaces/go"
)

const (
	ConsentPrices = "prices-v1"
	MaxOwnReviews = 10
	MaxReviewLen  = 1000
)

var (
	errConsent   = errors.New("consent_required")
	errOwnLimit  = errors.New("own_limit")
	errBadReview = errors.New("bad_review")
)

// Review is what the page and the model see: no author name, personal data masked.
type Review struct {
	ID          string `json:"id"`
	ProductID   string `json:"product_id"`
	ProductName string `json:"product_name"`
	Rating      int    `json:"rating"`
	Text        string `json:"text"`
	CreatedAt   string `json:"created_at"`
	Answered    bool   `json:"answered"`
	AnswerText  string `json:"answer_text"`
	Own         bool   `json:"own"`    // pasted by the visitor
	Masked      int    `json:"masked"` // personal data fragments hidden
	Draft       *Draft `json:"draft,omitempty"`
}

// Consent is the visitor's prices-v1 consent (kept for the session's lifetime: texts live only as long).
type Consent struct {
	Version string `json:"version"`
	At      string `json:"at"`
}

// deidentify masks personal data (names incl. the known author, phones, emails, ...) and shows a neutral marker.
func deidentify(text string, known ...string) (string, int) {
	var names []string
	for _, n := range known {
		if n = strings.TrimSpace(n); n != "" {
			names = append(names, n)
		}
	}
	r := ai.MaskPII(ai.MaskInput{Text: text, KnownNames: names})
	out := r.Text
	for _, e := range r.Entities {
		out = strings.ReplaceAll(out, e.Placeholder, "[скрыто]")
	}
	return out, len(r.Entities)
}

func (s *Session) fromMarketplace(r marketplaces.Review, lang string) Review {
	text, n := deidentify(r.Text, r.Author)
	ans, m := deidentify(r.AnswerText, r.Author)
	v := Review{ID: r.ID, ProductID: r.ProductID, Rating: r.Rating, Text: text, CreatedAt: r.CreatedAt,
		Answered: r.Answered, AnswerText: ans, Masked: n + m}
	if p := s.product(r.ProductID); p != nil {
		v.ProductName = p.Name(lang)
	}
	return v
}

// Reviews: unanswered first, newest first.
func (s *Session) Reviews(ctx context.Context, lang string) ([]Review, error) {
	rs, err := s.mp.ListReviews(ctx, marketplaces.ListReviewsInput{Limit: 100})
	if err != nil {
		return nil, err
	}
	out := []Review{}
	for _, r := range rs {
		out = append(out, s.fromMarketplace(r, lang))
	}
	for _, r := range s.own {
		v := *r
		if p := s.product(v.ProductID); p != nil {
			v.ProductName = p.Name(lang)
		}
		out = append(out, v)
	}
	for i := range out {
		if d, ok := s.drafts[out[i].ID]; ok && !out[i].Answered {
			dd := d
			out[i].Draft = &dd
		}
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && less(out[j], out[j-1]); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}

func less(a, b Review) bool {
	if a.Answered != b.Answered {
		return !a.Answered
	}
	return a.CreatedAt > b.CreatedAt
}

func (s *Session) review(ctx context.Context, id string) (*Review, error) {
	rs, err := s.Reviews(ctx, "ru")
	if err != nil {
		return nil, err
	}
	for i := range rs {
		if rs[i].ID == id {
			return &rs[i], nil
		}
	}
	return nil, errNotFound
}

// AddOwnReview stores a review the visitor pasted (only with the prices-v1 consent); the text is de-identified first.
func (s *Session) AddOwnReview(productID string, rating int, text, consent string, now time.Time) (*Review, error) {
	if consent != ConsentPrices {
		return nil, errConsent
	}
	text = strings.TrimSpace(text)
	if s.product(productID) == nil || rating < 1 || rating > 5 || text == "" || utf8.RuneCountInString(text) > MaxReviewLen {
		return nil, errBadReview
	}
	if len(s.own) >= MaxOwnReviews {
		return nil, errOwnLimit
	}
	s.consent = &Consent{Version: consent, At: now.UTC().Format(time.RFC3339)}
	masked, n := deidentify(text)
	s.nOwn++
	r := &Review{ID: fmt.Sprintf("own_%d", s.nOwn), ProductID: productID, Rating: rating, Text: masked,
		CreatedAt: now.UTC().Format(time.RFC3339), Own: true, Masked: n}
	s.own = append(s.own, r)
	return r, nil
}

// ReplyGen produces a reply draft (ai.Client.ReplyToReview, possibly wrapped in an accounts live call).
type ReplyGen func(ctx context.Context, in ai.ReviewReplyInput) (*ai.ReviewReply, error)

// DraftReply asks the ai module (reply_to_review) for a draft of the de-identified review: YandexGPT in the RU
// edition when the visitor is signed in and has live calls left, otherwise the deterministic mock.
func (s *Session) DraftReply(ctx context.Context, gen ReplyGen, reviewID string) (*Draft, error) {
	r, err := s.review(ctx, reviewID)
	if err != nil {
		return nil, err
	}
	lang, shop := "ru", "Домашняя лавка" // the demo seller's store name
	if !isCyrillic(r.Text) {
		lang, shop = "en", "Home Goods Shop"
	}
	product := ""
	if p := s.product(r.ProductID); p != nil {
		product = p.Name(lang)
	}
	if gen == nil {
		gen = ai.NewMock().ReplyToReview
	}
	res, err := gen(ctx, ai.ReviewReplyInput{Text: r.Text, Rating: r.Rating, Product: product, Shop: shop,
		Language: lang, Tone: "friendly", MaxChars: 600})
	if err != nil {
		return nil, err
	}
	d := Draft{Text: strings.TrimSpace(res.Text), Provider: res.Provider, Model: res.Model}
	s.drafts[r.ID] = d
	return &d, nil
}

// Answer publishes the reply: marketplace reviews through the marketplaces module (mock), own reviews are kept here.
func (s *Session) Answer(ctx context.Context, reviewID, text string) error {
	for _, r := range s.own {
		if r.ID == reviewID {
			r.Answered, r.AnswerText = true, text
			delete(s.drafts, reviewID)
			return nil
		}
	}
	if _, err := s.mp.AnswerReview(ctx, marketplaces.AnswerReviewInput{ReviewID: reviewID, Text: text}); err != nil {
		return err
	}
	delete(s.drafts, reviewID)
	return nil
}

func isCyrillic(s string) bool {
	for _, r := range s {
		if r >= 'А' && r <= 'я' || r == 'ё' || r == 'Ё' {
			return true
		}
	}
	return false
}
