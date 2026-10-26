---
title: Job Finder Agent Documentation
type: reference
owner: hhollenstain
status: draft
tags: [agent, job-finder, documentation, technical]
verified_at: null
review_interval: 90
---

# Job Finder Agent

The Job Finder agent is a local Go application designed to automate job searching by monitoring multiple sources and applying user-defined rules to filter and score opportunities.

## Overview

The Job Finder agent searches various job sources (remotive, remoteok, arbeitnow, linkedin, etc.) and applies filtering rules defined in your profile. Matched jobs are written to a local folder or synced to a KiwiFS knowledge base for further analysis.

## Getting Started

### Installation

1. Clone the repository
2. Install Go 1.24 or later
3. Run the application with:
   ```bash
   go run ./cmd/jobfinder
   ```

### Quick Start

1. Launch the application:
   ```bash
   go run ./cmd/jobfinder
   ```
   
2. Open [http://127.0.0.1:8787](http://127.0.0.1:8787) in your browser

3. The first launch creates `data/jobfinder.db` and enables Remotive, Remote OK, Arbeitnow, and public LinkedIn search

4. Copy `config.example.yaml` to `config.yaml` before the first launch if you want different seed values

### Running the Application

```bash
go build -o jobfinder ./cmd/jobfinder
./jobfinder -addr 127.0.0.1:8787 -data ./data
```

The binary is pure Go. `CGO_ENABLED=0` is fine.

## How It Works

### Search Process

1. **Fetches each enabled source** - Public feeds are cached for the number of hours set under Model
2. **Applies your profile as hard rules** - keywords, must-haves, excludes, remote, location, minimum pay, employment type
3. **Optionally scores the survivors with the model** - A score under your minimum is filtered out
4. **Optionally asks the model to add up to three public sources** when fewer than eight roles pass the rules
5. **Writes a markdown context pack** to `data/context/job-search/`

### Job Sources

| Kind | Needs a key | Notes |
| --- | --- | --- |
| Remotive, Remote OK, Arbeitnow | No | Public feeds. Listings link back to the source. |
| Greenhouse, Lever, Ashby | No | One company board. Use the public slug, such as `stripe`. |
| RSS / Atom, JSON | No | Any feed you are allowed to read. JSON uses dot paths (`jobs`, `title`, `url`). |
| Adzuna, USAJOBS | Yes | Official APIs. |
| Slack | Bot token | Channel IDs. The bot must already be in the channel (`channels:history`, and `groups:history` for private channels). |
| LinkedIn | No | Public guest job search. No account. Listings link back to the LinkedIn posting. |
| Indeed, Dice | RapidAPI JSearch key | JSearch returns postings it already aggregates. Filter with publishers `Indeed` or `Dice`. |

> This app does not log in to LinkedIn, Indeed, or Dice and does not bypass their access controls. LinkedIn search uses the public guest listings. Indeed and Dice stay behind an optional JSearch key.

### Models

| Provider | Endpoint |
| --- | --- |
| Ollama | OpenAI-compatible chat at `http://127.0.0.1:11434/v1`. No key. |
| Claude | Anthropic Messages API. |
| OpenAI | `https://api.openai.com/v1`. |
| Cursor | An OpenAI-compatible base URL plus that gateway's key. Cursor does not publish a general chat API for third-party apps. |
| Other | Any OpenAI-compatible `/v1/chat/completions` server. |

## Configuration

### Profile Settings

The profile defines your job search criteria:

```yaml
profile:
  notes: |
    Backend engineer. Prefer Go, a product the company can explain, and a role
    that says what the team actually owns.
  keywords: [go, golang, backend]
  must_have: []
  exclude: [unpaid, internship]
  locations: []
  remote_only: false
  include_remote: true
  min_salary: 0
  min_score: 70
  employment_types: []
```

### Settings

```yaml
settings:
  schedule: ""
  timezone: Local
  score_with_llm: false
  auto_research: false
  expand_sources: false
  max_llm_jobs: 25
  cache_hours: 6
  llm:
    provider: ollama
    base_url: http://127.0.0.1:11434/v1
    model: llama3.2
    api_key: ""
  kiwi:
    enabled: false
    base_url: http://127.0.0.1:3333
    api_key: ""
    space: ""
    path_prefix: job-search
```

## API Endpoints

The UI uses JSON under `/api`:
- `profile`
- `settings`
- `schedule`
- `sources`
- `source-kinds`
- `jobs`
- `runs`
- `context`

`POST /api/runs` starts a search and returns immediately. Poll `GET /api/runs/{id}` until `status` is no longer `running`.

## KiwiFS Integration

Point Context at a running server, usually `http://127.0.0.1:3333`. Writes use `PUT /api/kiwi/file` with `X-Actor: agent:job-finder`. Set a space and a bearer token when the server requires them.

Pages land under the path prefix (`job-search` by default): `profile.md`, `jobs/`, `companies/`, `runs/`, and `index.md`.

If KiwiFS is unreachable, the local folder is still written and the sync status says why the mirror failed.

## Development

### Code Structure

The project follows a standard Go structure:
- `cmd/jobfinder` - Main application entry point
- `internal/*` - Internal packages containing business logic
- `web/*` - Web UI components
- `config.example.yaml` - Example configuration file

### Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests
5. Submit a pull request

## Troubleshooting

### Common Issues

- **No jobs found**: Check that sources are enabled in the web UI
- **Connection problems**: Verify API keys and network connectivity
- **Performance issues**: Consider reducing cache hours or limiting sources

### Logs

The application logs information to stdout. Check for any error messages that might indicate what's going wrong.