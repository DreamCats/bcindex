package indexer

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/DreamCats/bcindex/internal/ast"
	"github.com/DreamCats/bcindex/internal/config"
	"github.com/DreamCats/bcindex/internal/embedding"
	"github.com/DreamCats/bcindex/internal/semantic"
	"github.com/DreamCats/bcindex/internal/store"
)

// Indexer handles the complete indexing pipeline
type Indexer struct {
	cfg            *config.Config
	db             *store.DB
	pipeline       *ast.Pipeline
	embedService   *embedding.Service
	semanticGen    *semantic.Generator
	symbolStore    *store.SymbolStore
	packageStore   *store.PackageStore
	edgeStore      *store.EdgeStore
	vectorStore    *store.VectorStore
}

// NewIndexer creates a new indexer
func NewIndexer(cfg *config.Config) (*Indexer, error) {
	// Open database
	db, err := store.Open(cfg.Database.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create embedding service
	embedService, err := embedding.NewService(&cfg.Embedding)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create embedding service: %w", err)
	}

	// Create stores
	symbolStore := store.NewSymbolStore(db)
	packageStore := store.NewPackageStore(db)
	edgeStore := store.NewEdgeStore(db)
	vectorStore := store.NewVectorStore(db)

	return &Indexer{
		cfg:          cfg,
		db:           db,
		pipeline:     ast.NewPipeline(),
		embedService: embedService,
		semanticGen:  semantic.NewGenerator(),
		symbolStore:  symbolStore,
		packageStore: packageStore,
		edgeStore:    edgeStore,
		vectorStore:  vectorStore,
	}, nil
}

// IndexRepository indexes a repository with embeddings
func (idx *Indexer) IndexRepository(ctx context.Context, repoPath string) error {
	startTime := time.Now()

	// Step 1: Extract symbols and relations
	log.Printf("Extracting symbols and relations from %s", repoPath)
	symbols, edges, err := idx.pipeline.ExtractRepositoryWithRelations(repoPath)
	if err != nil {
		return fmt.Errorf("failed to extract repository: %w", err)
	}
	log.Printf("Extracted %d symbols and %d relations", len(symbols), len(edges))

	// Step 2: Generate semantic descriptions and prepare for embedding
	log.Printf("Generating semantic descriptions")
	symbolData := idx.prepareSymbols(symbols)
	packageData := idx.preparePackages(symbols)

	// Step 3: Store symbols in database
	log.Printf("Storing symbols in database")
	if err := idx.symbolStore.CreateBatch(symbolData); err != nil {
		return fmt.Errorf("failed to store symbols: %w", err)
	}

	// Step 4: Store packages in database
	log.Printf("Storing packages in database")
	for _, pkg := range packageData {
		if err := idx.packageStore.Create(pkg); err != nil {
			log.Printf("Warning: failed to store package %s: %v", pkg.Path, err)
		}
	}

	// Step 5: Store edges
	log.Printf("Storing edges in database")
	storeEdges := idx.convertEdges(edges)
	if err := idx.edgeStore.CreateBatch(storeEdges); err != nil {
		return fmt.Errorf("failed to store edges: %w", err)
	}

	// Step 6: Generate and store embeddings
	log.Printf("Generating embeddings")
	if err := idx.indexEmbeddings(ctx, symbols); err != nil {
		return fmt.Errorf("failed to generate embeddings: %w", err)
	}

	duration := time.Since(startTime)
	log.Printf("Indexing completed in %v", duration)

	return nil
}

// prepareSymbols converts extracted symbols to store symbols with semantic text
func (idx *Indexer) prepareSymbols(extracted []*ast.ExtractedSymbol) []*store.Symbol {
	symbols := make([]*store.Symbol, len(extracted))

	// Group symbols by package for semantic generation
	pkgSymbols := make(map[string][]*ast.ExtractedSymbol)
	pkgImports := make(map[string][]string)

	for _, sym := range extracted {
		if sym.Kind == "package" {
			pkgSymbols[sym.PackagePath] = []*ast.ExtractedSymbol{sym}
			pkgImports[sym.PackagePath] = sym.Imports
		}
	}

	for _, sym := range extracted {
		if sym.Kind != "package" {
			pkgSymbols[sym.PackagePath] = append(pkgSymbols[sym.PackagePath], sym)
		}
	}

	// Generate semantic text for each symbol
	for i, sym := range extracted {
		symbols[i] = &store.Symbol{
			ID:          sym.ID,
			RepoPath:    idx.cfg.Repo.Path,
			Kind:        sym.Kind,
			PackagePath: sym.PackagePath,
			PackageName: sym.PackageName,
			Name:        sym.Name,
			Signature:   sym.Signature,
			FilePath:    sym.FilePath,
			LineStart:   sym.LineStart,
			LineEnd:     sym.LineEnd,
			DocComment:  sym.DocComment,
			Exported:    sym.Exported,
		}

		// Generate semantic text
		if sym.Kind == "package" {
			pkgSym := pkgSymbols[sym.PackagePath]
			if len(pkgSym) > 0 {
				symbols[i].SemanticText = idx.semanticGen.GeneratePackageCard(
					sym,
					pkgSym,
					pkgImports[sym.PackagePath],
				)
			}
		} else {
			// Get package card for context
			var pkgCard string
			if pkgSyms, ok := pkgSymbols[sym.PackagePath]; ok && len(pkgSyms) > 0 {
				// Find the package symbol
				for _, ps := range pkgSyms {
					if ps.Kind == "package" {
						pkgCard = idx.semanticGen.GeneratePackageCard(
							ps,
							pkgSyms,
							pkgImports[sym.PackagePath],
						)
						break
					}
				}
			}
			symbols[i].SemanticText = idx.semanticGen.GenerateSymbolCard(sym, pkgCard)
		}

		// Extract keywords from name and signature
		symbols[i].Tokens = idx.extractKeywords(sym)
	}

	return symbols
}

// preparePackages prepares package records
func (idx *Indexer) preparePackages(symbols []*ast.ExtractedSymbol) []*store.Package {
	pkgMap := make(map[string]*store.Package)

	// Group symbols by package and generate semantic cards
	pkgSymbols := make(map[string][]*ast.ExtractedSymbol)
	pkgImports := make(map[string][]string)

	for _, sym := range symbols {
		if sym.Kind == "package" {
			pkgSymbols[sym.PackagePath] = []*ast.ExtractedSymbol{sym}
			pkgImports[sym.PackagePath] = sym.Imports
		}
	}

	for _, sym := range symbols {
		if sym.Kind != "package" {
			pkgSymbols[sym.PackagePath] = append(pkgSymbols[sym.PackagePath], sym)
		}
	}

	for _, sym := range symbols {
		if sym.Kind != "package" {
			continue
		}

		// Generate semantic text for package
		pkgSym := pkgSymbols[sym.PackagePath]
		var summary string
		if len(pkgSym) > 0 {
			summary = idx.semanticGen.GeneratePackageCard(
				sym,
				pkgSym,
				pkgImports[sym.PackagePath],
			)
		}

		pkg := &store.Package{
			Path:        sym.PackagePath,
			Name:        sym.PackageName,
			RepoPath:    idx.cfg.Repo.Path,
			Summary:     summary,
			FileCount:   0, // Will be updated if we track files
			SymbolCount: 0,
		}

		// Count symbols in this package
		for _, s := range symbols {
			if s.PackagePath == sym.PackagePath {
				pkg.SymbolCount++
			}
		}

		pkgMap[sym.PackagePath] = pkg
	}

	// Convert map to slice
	packages := make([]*store.Package, 0, len(pkgMap))
	for _, pkg := range pkgMap {
		packages = append(packages, pkg)
	}

	return packages
}

// indexEmbeddings generates and stores embeddings for symbols
func (idx *Indexer) indexEmbeddings(ctx context.Context, symbols []*ast.ExtractedSymbol) error {
	// Filter symbols that should be embedded (skip packages and files)
	toEmbed := make([]*ast.ExtractedSymbol, 0)
	for _, sym := range symbols {
		if sym.Kind == "func" || sym.Kind == "method" ||
		   sym.Kind == "struct" || sym.Kind == "interface" {
			toEmbed = append(toEmbed, sym)
		}
	}

	if len(toEmbed) == 0 {
		return nil
	}

	// Get symbol IDs and prepare semantic texts
	// We need to retrieve the symbols from the store to get the semantic text
	symbolIDs := make([]string, len(toEmbed))
	texts := make([]string, len(toEmbed))

	for i, sym := range toEmbed {
		symbolIDs[i] = sym.ID
		// Use signature + doc for embedding (semantic text is in the store, not ExtractedSymbol)
		text := sym.Signature
		if sym.DocComment != "" {
			text += "\n" + sym.DocComment
		}
		texts[i] = text
	}

	// Generate embeddings in batch
	log.Printf("Generating embeddings for %d symbols", len(texts))
	embeddings, err := idx.embedService.EmbedBatch(ctx, texts)
	if err != nil {
		return fmt.Errorf("failed to generate embeddings: %w", err)
	}

	// Store embeddings
	log.Printf("Storing embeddings")
	if err := idx.vectorStore.InsertBatch(symbolIDs, embeddings, idx.cfg.Embedding.Model); err != nil {
		return fmt.Errorf("failed to store embeddings: %w", err)
	}

	return nil
}

// convertEdges converts ast.Edge to store.Edge
func (idx *Indexer) convertEdges(astEdges []*ast.Edge) []*store.Edge {
	edges := make([]*store.Edge, len(astEdges))
	for i, e := range astEdges {
		edges[i] = &store.Edge{
			FromID:     e.FromID,
			ToID:       e.ToID,
			EdgeType:   e.EdgeType,
			Weight:     e.Weight,
			ImportPath: e.ImportPath,
			CreatedAt:  time.Now(),
		}
	}
	return edges
}

// extractKeywords extracts keywords from a symbol
func (idx *Indexer) extractKeywords(sym *ast.ExtractedSymbol) []string {
	keywords := make([]string, 0)

	// Add name
	if sym.Name != "" {
		keywords = append(keywords, sym.Name)
	}

	// Add kind
	if sym.Kind != "" {
		keywords = append(keywords, sym.Kind)
	}

	// TODO: Add more sophisticated keyword extraction

	return keywords
}

// Close closes the indexer and releases resources
func (idx *Indexer) Close() error {
	return idx.db.Close()
}

// GetStores returns the stores for direct access
func (idx *Indexer) GetStores() (*store.SymbolStore, *store.PackageStore, *store.EdgeStore, *store.VectorStore) {
	return idx.symbolStore, idx.packageStore, idx.edgeStore, idx.vectorStore
}

// GetEmbedService returns the embedding service
func (idx *Indexer) GetEmbedService() *embedding.Service {
	return idx.embedService
}
