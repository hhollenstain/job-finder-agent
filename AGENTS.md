# Agents

This project uses a sophisticated agent-based architecture to search for and process job listings. The agents are responsible for fetching job data from various sources, filtering it according to user profiles, scoring relevant opportunities, and conducting research on promising candidates.

## Agent Components

### Main Agent Structure
The core `Agent` structure coordinates all operations, managing the interaction between various components including:
- Data storage (via `store`)
- Job sources (via `source`)
- Filtering logic (via `filter`)
- LLM-based scoring systems (via `llm`)
- Context management and research (via `kiwi`)

### Core Operations

1. **Fetch** - Retrieves job listings from configured sources using `fetchAll` and `fetchOne`
2. **Filter** - Applies user-defined profile criteria to determine initial matches
3. **Expand** - Optionally expands search to additional sources based on initial results
4. **Score** - Uses LLM-based systems to rank promising job opportunities
5. **Research** - Performs in-depth analysis of selected jobs using public information

## Agent Lifecycle

The agent operates through a well-defined lifecycle:
1. Start execution with `Start()` method
2. Fetch all available job listings from configured sources
3. Filter initial results based on user profile criteria
4. Optionally expand search to additional sources
5. Score promising candidates using LLM-based system 
6. Research selected opportunities for deeper insights
7. Store results and update context

## Key Agent Functions

### Agent Structure
```go
type Agent struct {
    store   *store.Store
    dataDir string
    mu      sync.Mutex
    running bool
}
```

### Main Execution Flow
- `Start()` - Initiates a new search run with trigger information
- `execute()` - Runs the complete search lifecycle (fetch → filter → score → research → store)
- `fetchAll()` - Concurrently fetches from multiple sources
- `fetchOne()` - Fetches from a single source with caching

### Scoring and Filtering
- `score()` - Uses LLM to score job opportunities based on profile criteria
- `ResearchJob()` - Conducts in-depth research on selected jobs using public information
- `autoResearch()` - Automatically conducts research on new matches
- `rankCandidate()` - Ranks candidates for optimal scoring evaluation

### Context Management
The agent integrates with a Kiwi context system to provide additional workspace knowledge during LLM interactions, enhancing the quality of job recommendations.

## Decision Making Process

The agent implements a multi-layered approach to decision making:
1. Initial filtering based on user profile criteria
2. LLM-based scoring for promising candidates (configurable threshold)
3. Optional research for deeper insights into top matches
4. Final storage and context updating

This architecture allows for flexible job searching with deep analysis capabilities, leveraging both traditional filtering approaches and advanced AI-driven scoring systems.