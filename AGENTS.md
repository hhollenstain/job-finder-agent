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

### Agent Lifecycle

The Job Finder agent operates through a carefully orchestrated 8-step execution process:

1. **Initialize Run** - Creates a new run record in the database with start time and trigger information
2. **Fetch Jobs** - Concurrently retrieves job postings from all enabled sources using caching to avoid redundant API calls  
3. **Filter Candidates** - Applies user-defined profile criteria to determine which jobs meet basic requirements
4. **Score Opportunities** - When enabled, uses LLMs to assign quality scores to promising candidates based on profile alignment
5. **Expand Sources** - Analyzes results and automatically adds up to 3 new public sources when fewer than 8 roles pass filtering rules  
6. **Store Results** - Persists matched jobs to the local database with full context information including scores, pros/cons, and fit summaries
7. **Research Promising Jobs** - When enabled, performs additional analysis on scored jobs to gather more detailed information
8. **Sync Context** - Synchronizes results to KiwiFS knowledge base (if configured) for persistent storage and long-term analysis

### Job Sources

The Job Finder agent supports many job sources. Each source type has different requirements for configuration, including whether an API key is needed.

| Kind | Needs a key | Notes |
| --- | --- | --- |
| Remotive | No | Public feed. Listings link back to the source. [API Documentation](https://remotive.com/api) |
| Remote OK | No | Public feed. Listings link back to the source. [API Documentation](https://remoteok.io/api) |
| Arbeitnow | No | Public feed. Listings link back to the source. [API Documentation](https://www.arbeitnow.io/) |
| Greenhouse | No | One company board. Use the public slug, such as `stripe`. |
| Lever | No | One company board. Use the public site name, such as `netflix`. |
| Ashby | No | One company board. Use the public board name, such as `notion`. |
| RSS / Atom | No | Any feed you are allowed to read. |
| JSON | No | Any JSON document you can describe with field paths. |
| Adzuna | Yes | Official API: [https://developer.adzuna.com/](https://developer.adzuna.com/)  
Required configuration keys: `app_id` and `app_key` (both needed). |
| USAJOBS | Yes | Official API: [https://developer.usajobs.gov/](https://developer.usajobs.gov/)  
Required configuration keys: `api_key` (Authorization-Key header) and `email` (User-Agent header). |
| Slack | Yes | Channel IDs. The bot must already be in the channel (`channels:history`, and `groups:history` for private channels). Required key: `token`. |
| LinkedIn | No | Public guest job search. No account required. Uses LinkedIn's public guest search API endpoint at `/jobs-guest/jobs/api/seeMoreJobPostings/search`. Listings link back to the LinkedIn posting. Official API documentation: [LinkedIn Jobs Guest Search](https://www.linkedin.com/jobs-guest/jobs/api/seeMoreJobPostings/search) |
| Indeed (via JSearch) | Yes | Official API: [https://rapidapi.com/rapiddweller/api/jsearch](https://rapidapi.com/rapiddweller/api/jsearch)  
Required configuration key: `api_key` (the API key from RapidAPI). |
| Dice (via JSearch) | Yes | Official API: [https://rapidapi.com/rapiddweller/api/jsearch](https://rapidapi.com/rapiddweller/api/jsearch)  
Required configuration key: `api_key` (the API key from RapidAPI). |
| jsearch | Yes | Official API: [https://rapidapi.com/rapiddweller/api/jsearch](https://rapidapi.com/rapiddweller/api/jsearch)  
Required configuration key: `api_key` (the API key from RapidAPI). |

> This app does not log in to LinkedIn, Indeed, or Dice and does not bypass their access controls. LinkedIn search uses the public guest listings. Indeed and Dice stay behind an optional JSearch key.

## API Endpoints

The agent exposes a RESTful API at `/api/*`:

### Profile Management
- `GET /api/profile` - Retrieves the current job search profile
- `PUT /api/profile` - Updates the job search profile

### Settings Management  
- `GET /api/settings` - Retrieves current application settings
- `PUT /api/settings` - Updates application settings

### Schedule Management
- `GET /api/schedule` - Retrieves the current scheduling configuration
- `PUT /api/schedule` - Updates the scheduling configuration

### Source Management
- `GET /api/sources` - Lists all configured job sources
- `POST /api/sources` - Creates a new job source
- `PUT /api/sources/{id}` - Updates an existing job source
- `DELETE /api/sources/{id}` - Deletes a job source
- `GET /api/source-kinds` - Retrieves available source types

### Job Management
- `GET /api/jobs` - Lists jobs with optional filtering parameters
- `GET /api/jobs/{id}` - Retrieves a specific job by ID
- `PATCH /api/jobs/{id}` - Updates job properties (status or favorite)
- `POST /api/jobs/purge` - Removes old jobs that haven't been kept
- `POST /api/jobs/{id}/research` - Performs additional research on a specific job

### Run Management
- `POST /api/runs` - Starts a new job search run (returns immediately)
- `GET /api/runs` - Lists recent job search runs
- `GET /api/runs/{id}` - Retrieves details of a specific run

### Context Management
- `GET /api/context` - Retrieves context page preview from KiwiFS or local files
- `POST /api/context/sync` - Forces synchronization of context files to KiwiFS

### Model Management
- `POST /api/models` - Lists available models for the configured LLM provider

### System Information
- `GET /api/health` - Returns system health status
- `GET /api/overview` - Provides an overview of system state including counts, schedule, and LLM status

`POST /api/runs` starts a search and returns immediately. Poll `GET /api/runs/{id}` until `status` is no longer `running`.

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

## Conclusion

The Job Finder agent provides a comprehensive solution for automated job searching by aggregating listings from multiple sources, applying intelligent filtering based on user profiles, and offering advanced features like LLM-powered scoring and research capabilities. With support for both public and API-based job boards, including specialized sources like LinkedIn guest search, Indeed, Dice, and USAJOBS, users can customize their job search experience to match their specific requirements. The integration with KiwiFS enables persistent storage and long-term analysis of job search data, making the agent not just a tool for finding jobs but also for building a knowledge base around career development opportunities.

The agent's flexibility is demonstrated through its wide range of supported job sources, from general public boards like Remotive, Remote OK, and Arbeitnow to company-specific boards through Greenhouse, Lever, and Ashby, as well as specialized APIs such as Adzuna's international job aggregator and USAJOBS' federal job listings. For platforms that do not offer public search APIs, the agent seamlessly integrates with RapidAPI's JSearch service that accesses Indeed and Dice job listings.

With comprehensive API documentation and clear configuration requirements for each source type, users can easily set up their job search to include any combination of these sources according to their career needs and preferences. The inclusion of features like automated scheduling, AI-powered scoring and research, and flexible filtering based on profile criteria makes this a robust tool for modern job seekers who want both broad market coverage and personalized matching.

## Conclusion

The Job Finder agent provides a comprehensive solution for automated job searching by aggregating listings from multiple sources, applying intelligent filtering based on user profiles, and offering advanced features like LLM-powered scoring and research capabilities. With support for both public and API-based job boards, including specialized sources like LinkedIn guest search, Indeed, Dice, and USAJOBS, users can customize their job search experience to match their specific requirements. The integration with KiwiFS enables persistent storage and long-term analysis of job search data, making the agent not just a tool for finding jobs but also for building a knowledge base around career development opportunities.

The agent's flexibility is demonstrated through its wide range of supported job sources, from general public boards like Remotive, Remote OK, and Arbeitnow to company-specific boards through Greenhouse, Lever, and Ashby, as well as specialized APIs such as Adzuna's international job aggregator and USAJOBS' federal job listings. For platforms that do not offer public search APIs, the agent seamlessly integrates with RapidAPI's JSearch service that accesses Indeed and Dice job listings.

With comprehensive API documentation and clear configuration requirements for each source type, users can easily set up their job search to include any combination of these sources according to their career needs and preferences. The inclusion of features like automated scheduling, AI-powered scoring and research, and flexible filtering based on profile criteria makes this a robust tool for modern job seekers who want both broad market coverage and personalized matching.