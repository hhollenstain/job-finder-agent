# Agents

The agent system is responsible for searching, filtering, and scoring job listings. Each agent coordinates between data sources, the LLM model, and the context filesystem to find matching opportunities based on user profiles and settings.

## Agent Components

### Research
The research component fetches public job postings and generates detailed briefings using the LLM. It reads stored job postings, retrieves the full text from the source URL when available, and analyzes the content against the user's profile to produce a comprehensive briefing.

### Expand 
The expand component dynamically adds new sources when the current sources don't return enough qualified listings. It intelligently extends the search by identifying additional sources that might provide relevant opportunities.

### Score
The scoring component evaluates potential matches using machine learning capabilities. It processes job listings through a structured scoring system that considers:
- Match quality with user's profile (keywords, skills, requirements)
- Salary expectations and ranges  
- Company location and remote work options
- Job title and role descriptions
- Technical stack and experience level

### Sync
The sync component manages context files for different sources and keeps the local filesystem in sync with remote repositories.

## Job Processing Lifecycle

1. **Fetch**: Agents collect job listings from configured sources using concurrent fetching to improve performance
2. **Filter**: Initial filtering occurs based on user's profile settings (keywords, must-haves, exclusions, locations)
3. **Score** (Optional): For listings that pass the initial filter, agents apply LLM-based scoring using structured prompts and evaluation criteria
4. **Store**: Processed job listings are saved to the local database with status tracking

## Decision Making Based on Filters and Scoring

The agent applies a two-tiered decision process:

### Initial Filtering
Jobs first go through a rule-based filtering system that evaluates each listing against the user's profile:
- Keywords match (positive or negative)
- Required skills/qualifications  
- Excluded terms or requirements
- Location preferences (include or exclude)
- Remote work preferences
- Salary minimum expectations

The filter uses configurable settings to determine what constitutes a "pass" vs. "reject" for each job listing.

### LLM-Based Scoring  
For jobs that pass the initial filters, agents apply an optional LLM scoring system:
- Uses structured prompts to evaluate role fit quality 
- Scores on a 0-100 scale with detailed pros and cons
- Considers seniority level (engineer, architect, etc.) 
- Evaluates salary expectations relative to market rates
- Takes into account company location and remote work options
- Prioritizes jobs based on technical stack overlap

The scoring system is designed to focus processing resources on the highest-quality matches. Jobs are ranked by:
1. Technical title keywords (infrastructure, platform, SRE, DevOps)
2. Salary levels (with higher paying positions ranked higher) 
3. Remote work capability
4. Location preferences

### Final Decision
A job listing is considered a match if:
- It passes the initial filtering criteria  
- OR it meets or exceeds the configured minimum score threshold (if scoring is enabled)

The agent system balances thoroughness with efficiency by:
- Applying filters first to reduce the list size
- Scoring only the most promising candidates based on pre-defined ranking criteria
- Allowing users to configure their preference for either extensive filtering or LLM-based scoring

## Configuration Settings

Agents are controlled through user-defined settings including:
- `ScoreWithLLM` - Enable/disable LLM scoring of jobs
- `AutoResearch` - Automatically research job details
- `MaxLLMJobs` - Maximum number of jobs to score with LLM per run
- `ExpandSources` - Enable dynamic source expansion based on search results
- `CacheHours` - How long to cache sources before re-fetching
