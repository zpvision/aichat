package leads

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"site-assistant/internal/ai"
	"strings"
	"time"
)

func explicitIntent(text string) bool {
	v := strings.ToLower(strings.TrimSpace(text))
	for _, p := range []string{"хочу оставить заявку", "прошу связаться со мной", "i want to submit a request", "please contact me", "chci podat žádost", "prosím kontaktujte mě", "ich möchte eine anfrage senden", "bitte kontaktieren sie mich"} {
		if strings.HasPrefix(v, p) {
			return true
		}
	}
	return false
}
func (s *Service) Prepare(ctx context.Context, v ai.Scope, execution, message, content, language string) (ai.CollectionPreparation, error) {
	out := ai.CollectionPreparation{}
	d, out, process, e := s.Store.reserve(ctx, v, execution, message, language, explicitIntent(content))
	if e != nil || !process {
		return out, e
	}
	sources, e := s.Store.sources(ctx, v)
	if e != nil {
		return out, e
	}
	values := d.Values
	code := ""
	extractCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	select {
	case s.slots <- struct{}{}:
		if s.Extractor == nil {
			code = "lead_extraction_unavailable"
		} else {
			candidates, err := s.Extractor.Extract(extractCtx, ExtractInput{Fields: d.Snapshot.Fields, Messages: sources})
			if err == nil {
				values, err = proposals(d.Snapshot, d.Values, sources, candidates)
			}
			if err != nil {
				if ctx.Err() != nil {
					<-s.slots
					return out, ctx.Err()
				}
				code = "lead_extraction_invalid"
				if errors.Is(err, context.DeadlineExceeded) {
					code = "lead_extraction_timeout"
				}
				values = d.Values
			}
		}
		<-s.slots
	default:
		code = "lead_extraction_busy"
	}
	if e = ctx.Err(); e != nil {
		return out, e
	}
	out.Instructions, out.Question = collectionPrompt(d.Snapshot, values, language, code)
	out.Commit = func(ctx context.Context, tx pgx.Tx) error {
		return s.Store.publish(ctx, tx, v, d, values, code)
	}
	return out, nil
}

var labels = map[string][]string{"full_name": {"Имя", "Name", "Jméno", "Name"}, "phone": {"Телефон", "Phone", "Telefon", "Telefon"}, "email": {"Email", "Email", "E-mail", "E-Mail"}, "company_name": {"Компания", "Company", "Společnost", "Unternehmen"}, "inn": {"ИНН", "Russian INN", "Ruské INN", "Russische INN"}, "request_description": {"Описание обращения", "Request description", "Popis žádosti", "Beschreibung der Anfrage"}}

func label(f Field, lang string) string {
	if f.Labels[lang] != "" {
		return f.Labels[lang]
	}
	if f.Label != "" {
		return f.Label
	}
	i := 0
	switch lang {
	case "en":
		i = 1
	case "cs":
		i = 2
	case "de":
		i = 3
	}
	if v := labels[f.Key]; v != nil {
		return v[i]
	}
	return f.Key
}
func collectionPrompt(s Settings, values map[string]Value, lang, code string) (string, string) {
	if !locale(lang) {
		lang = "en"
	}
	known := map[string]any{}
	fields := []map[string]any{}
	for _, f := range s.Fields {
		if !f.Active {
			continue
		}
		fields = append(fields, map[string]any{"key": f.Key, "label": ai.ClipRunes(label(f, lang), 60), "type": f.Type, "description": ai.ClipRunes(f.Description, 60), "required": f.Required})
		if v, ok := values[f.Key]; ok {
			known[f.Key] = map[string]any{"value": ai.ClipRunes(string(v.Value), 80), "valid": v.Valid}
		}
	}
	missing := Missing(s, values)
	parts := []string{}
	for _, key := range missing {
		if len(parts) == 2 {
			break
		}
		if key == "$contact" {
			parts = append(parts, map[string]string{"ru": "телефон или email", "en": "phone or email", "cs": "telefon nebo e-mail", "de": "Telefon oder E-Mail"}[lang])
			continue
		}
		for _, f := range s.Fields {
			if f.Key == key {
				parts = append(parts, label(f, lang))
			}
		}
	}
	question := map[string]string{"ru": "Проверьте карточку заявки и подтвердите отправку.", "en": "Review the request card and confirm sending.", "cs": "Zkontrolujte kartu žádosti a potvrďte odeslání.", "de": "Prüfen Sie die Anfragekarte und bestätigen Sie das Senden."}[lang]
	if len(parts) > 0 {
		question = map[string]string{"ru": "Уточните, пожалуйста: ", "en": "Please provide: ", "cs": "Prosím doplňte: ", "de": "Bitte ergänzen Sie: "}[lang] + strings.Join(parts, ", ") + "."
	}
	if code != "" {
		question = map[string]string{"ru": "Не удалось обновить данные автоматически. Проверьте или исправьте их в карточке заявки.", "en": "Automatic extraction could not update the data. Review or edit the request card.", "cs": "Automatická extrakce data neaktualizovala. Zkontrolujte nebo upravte kartu žádosti.", "de": "Die automatische Extraktion konnte die Daten nicht aktualisieren. Prüfen oder bearbeiten Sie die Anfragekarte."}[lang]
	}
	b, _ := json.Marshal(map[string]any{"goal": ai.ClipRunes(s.Goal, 300), "fields": fields, "known_values": known, "missing": missing, "next_question": question, "rule": "Ask only missing configured fields, at most two. Do not ask known valid values again. Do not invent data or claim a lead was sent. The confirmation card, not the model, controls submission."})
	return string(b), question
}
