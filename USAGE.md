# Usage Examples

This page captures common scenarios for the `bcindex evidence` command and the kinds of signals it is meant to surface. Evidence currently returns a package-level summary (top packages, key symbols, and high-level hints). It is intended to answer “where should I look?” rather than “exact line references.”

## Scenario 1: Locate the flow entry points
- Query: `bcindex evidence "indexing entry"`
- Expect: `cmd/bcindex`, `internal/indexer`, `internal/ast` in top packages
- Why it helps: quickly spot the entry command and the core indexing pipeline

## Scenario 2: Find module ownership of a capability
- Query: `bcindex evidence "embedding generation"`
- Expect: `internal/embedding`, `internal/indexer` with key symbols like `EmbedBatch`, `GenerateEmbeddings`
- Why it helps: know which package owns embedding logic

## Scenario 3: Understand config loading
- Query: `bcindex evidence "config load"`
- Expect: `internal/config` with key symbols `Load`, `LoadFromFile`, `SaveToFile`
- Why it helps: identify the configuration API surface

## Scenario 4: Trace storage and data flow
- Query: `bcindex evidence "store symbols"`
- Expect: `internal/store`, `internal/indexer` with symbols like `InsertSymbols`, `DeleteByRepo`
- Why it helps: locate persistence and indexing side effects

## Scenario 5: Find search implementation
- Query: `bcindex evidence "search logic"`
- Expect: `internal/retrieval`, `cmd/bcindex` with `Search`, `HybridRetriever`
- Why it helps: pinpoint retrieval core and CLI entry

## Scenario 6: (Future) citation-level evidence
- Query: `bcindex evidence "LoadFromFile references"`
- Desired output: file + line references for call sites (not implemented yet)
- Why it helps: answer “who calls this?” precisely

## Scenario 7: (Future) testing coverage hints
- Query: `bcindex evidence "embedding tests"`
- Desired output: test files and functions covering embedding symbols
- Why it helps: assess coverage and find examples

## When to use search
`bcindex search` is for precise symbol lookups when you already know a name or keyword. It returns definitions with file paths and signatures, so you can jump directly to code.

- Find a definition: `bcindex search "outputJSON"`
- Disambiguate same names: `bcindex search "NewGenerator"`
- Locate an API surface: `bcindex search "EmbedBatch"`
- Include unexported symbols: `bcindex search -all "fooBar"`

Rule of thumb: evidence answers “where should I look?”, search answers “where exactly is it?”
