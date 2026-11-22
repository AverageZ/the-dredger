---
name: dredger-bookmarks
description: "Search the user's personal bookmark collection for prior art, references, and inspiration. Trigger on: brainstorming, starting a PoC or spike, choosing a library or technique, or when the user asks whether they have saved anything about a topic."
allowed-tools: ["Bash"]
---

# Dredger Bookmarks

The user keeps a curated, AI-enriched collection of saved links in `dredger`, a
local CLI. Every bookmark has a title, an LLM-written summary, and concept tags.
This is their own reading history — it is a far better source of ideas they will
actually like than a generic web search.

## When to use this

Reach for it *before* generating ideas from scratch:

- Starting a brainstorm, spike, or proof of concept
- About to recommend a library, framework, or technique
- The user asks "do I have anything saved about X?", "what was that article…",
  "have I read about…"
- Designing something in a domain the user collects heavily (AI/LLMs, web
  development, game development, developer tooling)

Do **not** use it for general factual lookups. It searches one person's saved
links, not the web.

## Checking availability

```bash
command -v dredger >/dev/null && dredger stats
```

If `dredger` is not on PATH, tell the user to run `make install` in the
the-dredger repo. Do not try to work around it.

## Usage

### 1. Learn the vocabulary first

The tag vocabulary is long-tail and specific to this user. Checking it before
searching turns a guess into a targeted query:

```bash
dredger tags --min-count 5
```

### 2. Search

```bash
dredger search <concept words> --limit 10
dredger search "wave function collapse" --json
dredger search rendering --tag graphics --limit 5
```

Useful flags: `--json` (stable machine-readable output), `--limit N`,
`--tag T` (boost a tag, repeatable), `--tag-strict` (require it), `--domain D`,
`--full` (complete summaries), `--include-pruned`.

### 3. Drill in

Search prints truncated summaries to stay cheap. For the whole entry:

```bash
dredger show 412
```

## How to query well

- **Use concepts, not sentences.** `procedural generation roguelike` works;
  `how do I make a roguelike level generator` does not. Extra words dilute the
  score because covering all query terms is what ranks a result highly.
- **Start broad, then narrow.** One or two words first; add terms to refine.
- **Try the user's vocabulary.** If a query comes back weak, the output suggests
  related tags — re-run with those. They reflect what the collection actually
  contains.
- **Search more than once.** Different phrasings surface different links, and
  each search is cheap.

## Reading the output

- Results are **ranked**, not just filtered. The leading number is a relevance
  score; treat the top few as the real answers.
- `No strong matches for "X" — closest N:` means nothing scored well and these
  are weak suggestions. Say so rather than presenting them as finds.
- `No matches` (or an empty JSON `results` array) means no lexical matches, not
  proof that no related bookmarks exist. Try the suggested tags or a broader query
  before reporting that nothing relevant turned up.
- Links not yet dredged may have no summary or tags; they still match on title
  and URL.

## Reporting back

Cite what you found with its title and URL so the user can open it. Prefer:

> You've got three saved on this — *Wave Function Collapse for Dungeons*
> (marian-blog) looks closest to what you're describing.

over dumping raw command output. If nothing relevant turned up, say that in one
line and continue with the task; do not pad the answer with weak matches.
