package source

import (
	"context"
	"encoding/xml"
	"strings"

	"github.com/henryhollenstain/job-finder-agent/internal/model"
	"github.com/henryhollenstain/job-finder-agent/internal/textutil"
)

func searchRSS(ctx context.Context, cfg map[string]string, q Query) ([]model.JobDraft, error) {
	body, err := getBytes(ctx, strings.TrimSpace(cfg["url"]), nil)
	if err != nil {
		return nil, err
	}
	return parseFeed(body, cfg, q.Limit)
}

func parseFeed(body []byte, cfg map[string]string, limit int) ([]model.JobDraft, error) {
	var rss struct {
		XMLName xml.Name `xml:"rss"`
		Channel struct {
			Title string `xml:"title"`
			Items []struct {
				Title string `xml:"title"`
				Link  string `xml:"link"`
				Desc  string `xml:"description"`
				Pub   string `xml:"pubDate"`
				GUID  string `xml:"guid"`
			} `xml:"item"`
		} `xml:"channel"`
	}
	if err := xml.Unmarshal(body, &rss); err == nil && len(rss.Channel.Items) > 0 {
		company := companyOr(cfg, rss.Channel.Title)
		out := make([]model.JobDraft, 0, len(rss.Channel.Items))
		for _, item := range rss.Channel.Items {
			id := strings.TrimSpace(item.GUID)
			if id == "" {
				id = item.Link
			}
			out = append(out, model.JobDraft{
				ExternalID:  id,
				Title:       textutil.StripHTML(item.Title),
				Company:     company,
				URL:         strings.TrimSpace(item.Link),
				Description: textutil.StripHTML(item.Desc),
				PostedAt:    textutil.ParseTime(item.Pub),
			})
			if len(out) >= limit && limit > 0 {
				break
			}
		}
		return out, nil
	}
	var atom struct {
		XMLName xml.Name `xml:"feed"`
		Title   string   `xml:"title"`
		Entries []struct {
			Title   string `xml:"title"`
			ID      string `xml:"id"`
			Updated string `xml:"updated"`
			Summary string `xml:"summary"`
			Content string `xml:"content"`
			Links   []struct {
				Href string `xml:"href,attr"`
				Rel  string `xml:"rel,attr"`
			} `xml:"link"`
		} `xml:"entry"`
	}
	if err := xml.Unmarshal(body, &atom); err != nil {
		return nil, err
	}
	company := companyOr(cfg, atom.Title)
	out := make([]model.JobDraft, 0, len(atom.Entries))
	for _, entry := range atom.Entries {
		link := ""
		for _, l := range entry.Links {
			if l.Rel == "" || l.Rel == "alternate" {
				link = l.Href
				break
			}
		}
		desc := entry.Content
		if desc == "" {
			desc = entry.Summary
		}
		out = append(out, model.JobDraft{
			ExternalID:  entry.ID,
			Title:       textutil.StripHTML(entry.Title),
			Company:     company,
			URL:         link,
			Description: textutil.StripHTML(desc),
			PostedAt:    textutil.ParseTime(entry.Updated),
		})
		if len(out) >= limit && limit > 0 {
			break
		}
	}
	return out, nil
}
