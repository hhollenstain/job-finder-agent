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

## Component Interactions

The agent components work together in a specific sequence:

1. **Fetch Phase**: The `fetchAll` function concurrently fetches jobs from all enabled sources. Each source is fetched in parallel using a semaphore to limit concurrent requests.

2. **Filter Phase**: After fetching, candidates are filtered using `filter.Decide()` based on the user profile criteria. This determines which jobs pass the initial filter and are considered for scoring.

3. **Expand Phase**: If enabled (`settings.ExpandSources`), the `expand` component analyzes the filtering results. When too few jobs pass the initial filter, it uses an LLM to suggest additional job sources that might be suitable for the user profile.

4. **Scoring Phase**: If LLM scoring is enabled (`settings.ScoreWithLLM`), the `score` component is invoked using `a.score()`. This scores all passing candidates (after filtering and potentially expansion) using LLM analysis of their fit with user profile. The scoring component prioritizes candidates based on rankCandidate function to optimize LLM usage.

5. **Storage Phase**: Filtered and scored jobs are saved to the local database through `a.store.UpsertJob()`. The results from this operation determine if new jobs were discovered or if duplicates were found.

6. **Research Phase**: If automatic research is enabled (`settings.AutoResearch`) and LLM scoring was used, `autoResearch` is called to perform detailed research on promising matches, enriching them with additional information. This happens after scoring but before synchronization.

7. **Sync Phase**: Finally, the `Sync()` function synchronizes all local database changes with external systems (like KiwiFS), ensuring persistence across agent runs.

The interaction logic is: fetch → filter → expand (if enabled) → score (if enabled) → store → research (if enabled) → sync.