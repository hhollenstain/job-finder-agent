package source

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/henryhollenstain/job-finder-agent/internal/model"
	"github.com/henryhollenstain/job-finder-agent/internal/textutil"
)

func searchJSON(ctx context.Context, cfg map[string]string, q Query) ([]model.JobDraft, error) {
	headers := map[string]string{}
	if token := strings.TrimSpace(cfg["authorization"]); token != "" {
		headers["Authorization"] = "Bearer " + token
	}
	body, err := getBytes(ctx, strings.TrimSpace(cfg["url"]), headers)
	if err != nil {
		return nil, err
	}
	var root any
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, err
	}
	items := dig(root, strings.TrimSpace(cfg["items_path"]))
	arr, ok := items.([]any)
	if !ok {
		for _, key := range []string{"jobs", "data", "results"} {
			if arr, ok = dig(root, key).([]any); ok {
				break
			}
		}
	}
	if !ok {
		return nil, errJSONShape
	}
	titlePath := cfgOr(cfg, "title", "title")
	companyPath := cfgOr(cfg, "company", "company")
	linkPath := cfgOr(cfg, "link", "url")
	descPath := cfgOr(cfg, "description", "description")
	locPath := cfgOr(cfg, "location", "location")
	out := make([]model.JobDraft, 0, len(arr))
	for i, item := range arr {
		title := asString(dig(item, titlePath))
		if title == "" {
			title = asString(dig(item, "name"))
		}
		if title == "" {
			continue
		}
		company := asString(dig(item, companyPath))
		if company == "" {
			company = asString(dig(item, "company_name"))
		}
		if company == "" {
			company = companyOr(cfg, "Unknown")
		}
		link := asString(dig(item, linkPath))
		if link == "" {
			link = asString(dig(item, "link"))
		}
		desc := textutil.StripHTML(asString(dig(item, descPath)))
		loc := asString(dig(item, locPath))
		id := asString(dig(item, "id"))
		if id == "" {
			id = link
		}
		if id == "" {
			id = strconv.Itoa(i)
		}
		var remote *bool
		if v, ok := dig(item, cfgOr(cfg, "remote", "remote")).(bool); ok {
			remote = &v
		}
		out = append(out, model.JobDraft{
			ExternalID:  id,
			Title:       title,
			Company:     company,
			Location:    loc,
			Remote:      remote,
			URL:         link,
			Description: desc,
		})
		if len(out) >= q.Limit {
			break
		}
	}
	return out, nil
}

var errJSONShape = errString("json feed has no job array; set items_path")

type errString string

func (e errString) Error() string { return string(e) }

func dig(v any, path string) any {
	if path == "" {
		return v
	}
	cur := v
	for _, part := range strings.Split(path, ".") {
		switch node := cur.(type) {
		case map[string]any:
			cur = node[part]
		case []any:
			i, err := strconv.Atoi(part)
			if err != nil || i < 0 || i >= len(node) {
				return nil
			}
			cur = node[i]
		default:
			return nil
		}
	}
	return cur
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}
