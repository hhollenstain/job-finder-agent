# Job Finder Agent

The job finder agent is responsible for automating the process of searching for jobs, filtering them based on user preferences, and optionally scoring and researching job postings.

## Architecture

The agent coordinates between multiple components:
- **Sources**: Job boards and feeds that provide job listings
- **Filter**: Decision engine that applies user-defined filters to determine job suitability
- **Score**: LLM-based scoring system for ranking job opportunities  
- **Research**: System for fetching additional information about jobs and generating briefings
- **Expand**: System for adding new sources when initial filtering is too restrictive
- **Sync**: System for synchronizing job data with external systems

## Main Agent Structure

The `Agent` struct is the main coordinator, responsible for:
- Managing the search lifecycle
- Fetching job listings from sources
- Filtering candidates based on user profile
- Optionally scoring and researching jobs
- Persisting results to storage
- Adding new sources when needed (expand functionality)

### Key Functions

- `Start(trigger string, force bool)`: Initiates a search run with optional force refresh
- `execute(ctx context.Context, run *model.Run, force bool)`: Main execution loop that coordinates all agent functions
- `fetchAll(ctx context.Context, sources []model.Source, profile model.Profile, settings model.Settings, force bool, notes *[]string)`: Fetches jobs from all configured sources in parallel  
- `fetchOne(ctx context.Context, src model.Source, profile model.Profile, ttl time.Duration, force bool)`: Fetches jobs from a single source
- `score(ctx context.Context, settings model.Settings, profile model.Profile, cands []candidate)`: Scores job candidates using LLM
- `autoResearch(ctx context.Context, settings model.Settings, profile model.Profile)`: Performs automatic research on top candidates
- `expand(ctx context.Context, settings model.Settings, profile model.Profile, existing []model.Source, passCount int)`: Adds new sources when filtering is too restrictive

## Agent Lifecycle

1. **Start**: Agent is initiated with trigger and force parameters
2. **Fetch**: Job listings are fetched from all configured sources (with caching)
3. **Filter**: Candidates are filtered against the user profile using the filter system  
4. **Score** (optional): If enabled, job postings are scored using LLM
5. **Expand** (optional): If enabled and few jobs pass filtering, the agent may add new sources
6. **Research** (optional): If enabled, additional research is performed on top candidates
7. **Store**: Final results are saved to persistent storage
8. **Sync**: Context is synchronized with external systems

## Components

### Research
Provides LLM-powered analysis of job listings, including:
- Company information
- Role requirements  
- Potential risks and considerations
- Questions to ask when applying

### Expand
Adds new sources to the search when the initial filtering produces a small set of results.
This uses an LLM to suggest additional job sources that might yield better matches.

### Score
Implements LLM-based scoring of job opportunities based on:
- User profile match
- Job requirements and responsibilities  
- Company characteristics
- Compensation details

### Sync
Handles synchronization of job data with external context systems (e.g., KiwiFS).

## Configuration

The agent respects settings defined in the configuration including:
- Cache duration for sources
- Whether to use LLM scoring
- Whether to perform automatic research
- Whether to automatically expand sources
- Maximum number of jobs to process with LLM
- Whether to enable remote job detection

## Usage Patterns

### Manual Search
```go
agent := agent.New(store, dataDir)
run, err := agent.Start("manual", false)
```

### Auto Search (with force refresh)
```go
agent := agent.New(store, dataDir)
run, err := agent.Start("auto", true)
```

## Job Processing

1. **Fetch**: All sources are queried in parallel and jobs are cached
2. **Filter**: Each job is evaluated against the user's profile  
3. **Score**: If configured, jobs are scored using an LLM
4. **Research**: When configured, additional information is fetched and analyzed for top candidates
5. **Store**: Final results are saved to persistent storage

The agent maintains a detailed trail of all operations in the `model.Run` structure for tracking search progress and outcomes.