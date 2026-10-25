# Job Finder

A local Go agent that searches the job sources you turn on, drops roles that miss your rules, and can score or brief the rest with Claude, OpenAI, Ollama, or any OpenAI-compatible endpoint. Matched roles are written to a folder you can point at [KiwiFS](https://docs.kiwifs.com/).

## Run

```bash
go run ./cmd/jobfinder
```

Open [http://127.0.0.1:8787](http://127.0.0.1:8787). The first launch creates `data/jobfinder.db` and enables Remotive, Remote OK, Arbeitnow, and public LinkedIn search. Copy `config.example.yaml` to `config.yaml` before that first launch if you want different seed values.

Starting the app does not search. Use **Run search**, or pass `-search` to run once at startup. A saved schedule runs only when that time arrives.

```bash
go build -o jobfinder ./cmd/jobfinder
./jobfinder -addr 127.0.0.1:8787 -data ./data
```

The binary is pure Go. `CGO_ENABLED=0` is fine.

## What a search does

1. Fetches each enabled source. Public feeds are cached for the number of hours set under Model. "Ignore cache" forces a refresh.
2. Applies your profile as hard rules: keywords (any), must-haves (all), excludes, remote, location, minimum pay, employment type. Unknown pay is kept. Pay is compared as published, with no currency conversion.
3. Optionally scores the survivors with the model. A score under your minimum is filtered out.
4. Optionally asks the model to add up to three public sources when fewer than eight roles pass the rules.
5. Writes a markdown context pack to `data/context/job-search/`. When KiwiFS is enabled, the same pages are `PUT` under that prefix. The workspace `SCHEMA.md` and `.kiwi/playbook.md` are not replaced.

Save and Dismiss stick. A later search will not flip those.

## Sources

| Kind | Needs a key | Notes |
| --- | --- | --- |
| Remotive, Remote OK, Arbeitnow | No | Public feeds. Listings link back to the source. |
| Greenhouse, Lever, Ashby | No | One company board. Use the public slug, such as `stripe`. |
| RSS / Atom, JSON | No | Any feed you are allowed to read. JSON uses dot paths (`jobs`, `title`, `url`). |
| Adzuna, USAJOBS | Yes | Official APIs. |
| Slack | Bot token | Channel IDs. The bot must already be in the channel (`channels:history`, and `groups:history` for private channels). |
| LinkedIn | No | Public guest job search. No account. Listings link back to the LinkedIn posting. |
| Indeed, Dice | RapidAPI JSearch key | JSearch returns postings it already aggregates. Filter with publishers `Indeed` or `Dice`. |

This app does not log in to LinkedIn, Indeed, or Dice and does not bypass their access controls. LinkedIn search uses the public guest listings. Indeed and Dice stay behind an optional JSearch key.

## Models

| Provider | Endpoint |
| --- | --- |
| Ollama | OpenAI-compatible chat at `http://127.0.0.1:11434/v1`. No key. |
| Claude | Anthropic Messages API. |
| OpenAI | `https://api.openai.com/v1`. |
| Cursor | An OpenAI-compatible base URL plus that gateway's key. Cursor does not publish a general chat API for third-party apps. |
| Other | Any OpenAI-compatible `/v1/chat/completions` server. |

Scoring and research stay off until you enable them. Research fetches the public posting page, strips it to text, and asks the model to separate what the posting says from anything else.

On first seed only, `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, and `OLLAMA_HOST` fill the matching provider when its key or host is still empty.

## Schedule

Five-field cron, plus names such as `@hourly` and `@daily`. Example: `0 8 * * 1-5` is weekdays at 08:00 in the timezone you set (`Local` or `America/Los_Angeles`). An empty expression means manual runs only. Launching the app does not run a missed schedule.

## KiwiFS

Point Context at a running server, usually `http://127.0.0.1:3333`. Writes use `PUT /api/kiwi/file` with `X-Actor: agent:job-finder`. Set a space and a bearer token when the server requires them. Pages land under the path prefix (`job-search` by default): `profile.md`, `jobs/`, `companies/`, `runs/`, and `index.md`.

If KiwiFS is unreachable, the local folder is still written and the sync status says why the mirror failed.

## API

The UI uses JSON under `/api`: `profile`, `settings`, `schedule`, `sources`, `source-kinds`, `jobs`, `runs`, and `context`. `POST /api/runs` starts a search and returns immediately. Poll `GET /api/runs/{id}` until `status` is no longer `running`.
