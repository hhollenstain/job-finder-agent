package source

import (
	"context"
	"regexp"
	"strings"

	"github.com/henryhollenstain/job-finder-agent/internal/model"
	"github.com/henryhollenstain/job-finder-agent/internal/textutil"
)

var urlRE = regexp.MustCompile(`https?://[^\s>]+`)

func searchSlack(ctx context.Context, cfg map[string]string, q Query) ([]model.JobDraft, error) {
	token := strings.TrimSpace(cfg["token"])
	term := strings.ToLower(searchTerms(cfg, q))
	var out []model.JobDraft
	for _, channel := range splitComma(cfg["channels"]) {
		u := "https://slack.com/api/conversations.history?channel=" + queryEscape(channel) + "&limit=100"
		var payload struct {
			OK       bool   `json:"ok"`
			Error    string `json:"error"`
			Messages []struct {
				Text    string `json:"text"`
				TS      string `json:"ts"`
				Subtype string `json:"subtype"`
			} `json:"messages"`
		}
		if err := getJSON(ctx, u, map[string]string{"Authorization": "Bearer " + token}, &payload); err != nil {
			return nil, err
		}
		if !payload.OK {
			return nil, errSlack(channel, payload.Error)
		}
		for _, msg := range payload.Messages {
			if msg.Subtype != "" && msg.Subtype != "bot_message" && msg.Subtype != "file_share" {
				continue
			}
			text := strings.TrimSpace(msg.Text)
			if text == "" {
				continue
			}
			if term != "" && !containsAnyWord(strings.ToLower(text), term) {
				continue
			}
			link := firstURL(text)
			if link == "" && len(text) < 40 {
				continue
			}
			title := firstLine(text)
			out = append(out, model.JobDraft{
				ExternalID:  channel + ":" + msg.TS,
				Title:       title,
				Company:     "Slack " + channel,
				Location:    "",
				URL:         link,
				Description: text,
				PostedAt:    textutil.ParseTime(slackTS(msg.TS)),
			})
			if len(out) >= q.Limit {
				return out, nil
			}
		}
	}
	return out, nil
}

func errSlack(channel, code string) error {
	if code == "" {
		code = "unknown"
	}
	return &slackError{channel: channel, code: code}
}

type slackError struct{ channel, code string }

func (e *slackError) Error() string {
	return "slack " + e.channel + ": " + e.code
}

func firstURL(text string) string {
	m := urlRE.FindString(text)
	return strings.TrimRight(m, ">).,]")
}

func firstLine(text string) string {
	line := text
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		line = text[:i]
	}
	line = strings.TrimSpace(line)
	if len(line) > 140 {
		line = line[:140]
	}
	if line == "" {
		return "Slack post"
	}
	return line
}

func slackTS(ts string) string {
	if i := strings.IndexByte(ts, '.'); i > 0 {
		return ts[:i]
	}
	return ts
}
