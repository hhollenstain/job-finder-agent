# Agent Architecture

The job-finder-agent is a sophisticated searching system that coordinates multiple components to find and evaluate job opportunities based on user profiles. The core of this system is the `Agent` struct, which orchestrates the entire job search process.

## Main Agent Structure

The `Agent` struct in `internal/agent/agent.go` is the central coordinator that manages:
- Job sourcing from various sources
- Filtering mechanisms based on user profiles
- LLM-powered scoring and research
- Data storage and synchronization with external systems

```go
type Agent struct {
    store   *store.Store
    dataDir string

    mu      sync.Mutex
    running bool
}
```

## Core Components

### Research
The `research` component fetches public job posting information and uses an LLM to analyze each position against the user's profile. This provides additional context that helps with decision-making.

Key functions:
- `ResearchJob(ctx context.Context, id string)`: Analyzes a specific job posting 
- `autoResearch(ctx context.Context, settings model.Settings, profile model.Profile)`: Automatically researches newly matched jobs

### Expand
The `expand` component uses LLMs to identify and add new job sources when the initial search results are too sparse. This helps diversify the job search and find opportunities that might have been missed.

Key functions:
- `expand(ctx context.Context, settings model.Settings, profile model.Profile, existing []model.Source, passCount int)`: Adds new sources based on profile criteria
- `sourceFromTool(raw string, now time.Time)`: Converts tool responses into source configurations

### Score
The `score` component applies LLM-powered scoring to evaluated job opportunities. This helps prioritize jobs that are most likely to be of interest to the user.

Key functions:
- `score(ctx context.Context, settings model.Settings, profile model.Profile, cands []candidate)`: Applies LLM-based scoring to candidates

### Sync
The `sync` component handles synchronization with external systems (like KiwiFS) to persist job data and maintain consistency across multiple runs.

Key function:
- `Sync(ctx context.Context)`: Synchronizes the local database with remote storage systems

## Agent Lifecycle

The agent lifecycle begins when `Start()` is called, which:
1. Records a new run in the database
2. Fetches jobs from configured sources (with caching)
3. Filters jobs based on user profile criteria
4. Optionally scores jobs using LLMs
5. Saves job data to the local store
6. Performs automatic research on promising matches  
7. Synchronizes with external systems

Each run is managed through a `Run` struct that tracks progress, status, and results.