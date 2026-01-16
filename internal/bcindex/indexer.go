package bcindex

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/blevesearch/bleve/v2"
)

type IndexWarning struct {
	Count   int
	Samples []string
}

func (w *IndexWarning) Error() string {
	if w == nil {
		return ""
	}
	if len(w.Samples) > 0 {
		return fmt.Sprintf("index completed with %d errors: %s", w.Count, strings.Join(w.Samples, "; "))
	}
	return fmt.Sprintf("index completed with %d errors", w.Count)
}

type indexErrorCollector struct {
	mu      sync.Mutex
	count   int
	samples []string
}

type vectorJob struct {
	rel      string
	content  []byte
	mdChunks []MDChunk
}

func newIndexErrorCollector() *indexErrorCollector {
	return &indexErrorCollector{}
}

func (c *indexErrorCollector) Add(path string, err error) {
	if err == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.count++
	if len(c.samples) < 5 {
		c.samples = append(c.samples, fmt.Sprintf("%s: %v", path, err))
	}
}

func (c *indexErrorCollector) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.count == 0 {
		return nil
	}
	return &IndexWarning{Count: c.count, Samples: c.samples}
}

func IndexRepo(root string) error {
	return IndexRepoWithOptions(root, nil, IndexOptions{})
}

func IndexRepoWithProgress(root string, reporter ProgressReporter) error {
	return IndexRepoWithOptions(root, reporter, IndexOptions{})
}

func IndexRepoWithOptions(root string, reporter ProgressReporter, opts IndexOptions) error {
	paths, meta, err := InitRepo(root)
	if err != nil {
		return err
	}
	tier, err := resolveIndexTierOption(opts)
	if err != nil {
		return err
	}

	cfg, _, err := LoadIndexConfigOptional()
	if err != nil {
		return err
	}
	_, err = InitFileFilter(cfg, paths.Root)
	if err != nil {
		return err
	}

	if err := os.RemoveAll(paths.TextDir); err != nil {
		return fmt.Errorf("reset text index: %w", err)
	}
	if err := os.RemoveAll(paths.SymbolDir); err != nil {
		return fmt.Errorf("reset symbol index: %w", err)
	}
	if err := ensureDir(paths.TextDir); err != nil {
		return err
	}
	if err := ensureDir(paths.SymbolDir); err != nil {
		return err
	}

	textIndex, err := CreateTextIndex(paths.TextDir)
	if err != nil {
		return err
	}
	defer textIndex.Close()

	store, err := OpenSymbolStore(symbolDBPath(paths))
	if err != nil {
		return err
	}
	defer store.Close()

	if err := store.InitSchema(true); err != nil {
		return err
	}

	vectorCfg, vectorEnabled, err := loadVectorConfigForIndex()
	if err != nil {
		return err
	}

	collector := newIndexErrorCollector()

	var pkgIndex *GoPackageIndex
	if tierAllowsGoList(tier) {
		pkgIndex, err = BuildGoPackageIndex(paths.Root)
		if err != nil {
			collector.Add("go_list", err)
			pkgIndex = nil
		} else {
			for _, rel := range pkgIndex.Depends {
				if err := store.InsertRelation(rel); err != nil {
					collector.Add("go_list", err)
					break
				}
			}
		}
	}

	files, err := listTrackedFiles(paths.Root)
	if err != nil {
		return err
	}

	indexable := make([]string, 0, len(files))
	for _, rel := range files {
		if shouldIndex(rel) {
			indexable = append(indexable, rel)
		}
	}
	if reporter != nil {
		announcePhase(reporter, fmt.Sprintf("phase:text+symbol files=%d", len(indexable)))
		reporter.Start(len(indexable))
		defer reporter.Finish()
	}

	var storeMu sync.Mutex
	var vectorRuntime *VectorRuntime
	var vectorJobs chan vectorJob
	var vectorWG sync.WaitGroup
	if vectorEnabled {
		runtime, err := NewVectorRuntime(vectorCfg)
		if err != nil {
			collector.Add("vector_runtime", err)
			vectorEnabled = false
		} else {
			vectorRuntime = runtime
			defer vectorRuntime.Close()
			ctx := context.Background()
			if err := vectorRuntime.EnsureCollection(ctx); err != nil {
				collector.Add("vector_runtime", err)
				vectorEnabled = false
			} else {
				if reporter != nil {
					mode := vectorStoreMode(vectorRuntime)
					announcePhase(reporter, fmt.Sprintf("phase:vector mode=%s workers=%d batch=%d", mode, vectorCfg.VectorWorkers, vectorCfg.VectorBatchSize))
				}
				if err := vectorRuntime.store.DeletePointsByRepo(ctx, vectorCfg.QdrantCollection, storeRepoID(paths.Root)); err != nil {
					collector.Add("vector_cleanup", err)
				}
				workers := vectorCfg.VectorWorkers
				if workers <= 0 {
					workers = 1
				}
				vectorJobs = make(chan vectorJob, workers*2)
				for i := 0; i < workers; i++ {
					vectorWG.Add(1)
					go func() {
						defer vectorWG.Done()
						for job := range vectorJobs {
							if err := upsertVectorChunks(paths.Root, store, job.rel, job.content, job.mdChunks, vectorRuntime, &storeMu); err != nil {
								collector.Add("vector:"+job.rel, err)
							}
						}
					}()
				}
			}
		}
	}
	indexCtx := IndexContext{Tier: tier}
	if pkgIndex != nil {
		indexCtx.DirImportPath = pkgIndex.DirToImportPath
	}

	for _, rel := range indexable {
		abs := filepath.Join(paths.Root, filepath.FromSlash(rel))
		content, err := os.ReadFile(abs)
		if err != nil {
			collector.Add(rel, err)
			if reporter != nil {
				reporter.Increment()
			}
			continue
		}
		ext := strings.ToLower(filepath.Ext(rel))
		switch ext {
		case ".go":
			storeMu.Lock()
			err := indexGoFile(store, textIndex, rel, content, &indexCtx)
			storeMu.Unlock()
			if err != nil {
				collector.Add(rel, err)
				if reporter != nil {
					reporter.Increment()
				}
				continue
			}
			if vectorJobs != nil {
				vectorJobs <- vectorJob{rel: rel, content: content}
			}
		case ".md", ".markdown":
			storeMu.Lock()
			err := indexMarkdownFile(store, textIndex, rel, content)
			storeMu.Unlock()
			if err != nil {
				collector.Add(rel, err)
				if reporter != nil {
					reporter.Increment()
				}
				continue
			}
			if vectorJobs != nil {
				mdChunks := ChunkMarkdown(content)
				vectorJobs <- vectorJob{rel: rel, content: content, mdChunks: mdChunks}
			}
		}
		storeMu.Lock()
		err = store.InsertFile(FileEntryFromContent(rel, ext, content))
		storeMu.Unlock()
		if err != nil {
			collector.Add(rel, err)
			if reporter != nil {
				reporter.Increment()
			}
			continue
		}
		if reporter != nil {
			reporter.Increment()
		}
	}
	if vectorJobs != nil {
		if reporter != nil {
			announcePhase(reporter, "phase:vector waiting")
		}
		close(vectorJobs)
		vectorWG.Wait()
		if reporter != nil {
			announcePhase(reporter, "phase:vector done")
		}
	}

	meta.LastIndexAt = time.Now()
	meta.UpdatedAt = time.Now()
	if err := SaveRepoMeta(paths, meta); err != nil {
		return err
	}
	return collector.Err()
}

func IndexRepoDelta(root string, changes []FileChange, reporter ProgressReporter) error {
	return IndexRepoDeltaWithOptions(root, changes, reporter, IndexOptions{})
}

func IndexRepoDeltaWithOptions(root string, changes []FileChange, reporter ProgressReporter, opts IndexOptions) error {
	paths, meta, err := InitRepo(root)
	if err != nil {
		return err
	}
	tier, err := resolveIndexTierOption(opts)
	if err != nil {
		return err
	}

	cfg, _, err := LoadIndexConfigOptional()
	if err != nil {
		return err
	}
	_, err = InitFileFilter(cfg, paths.Root)
	if err != nil {
		return err
	}

	if err := ensureIndex(paths, "mixed"); err != nil {
		return err
	}

	textIndex, err := OpenTextIndex(paths.TextDir)
	if err != nil {
		return err
	}
	defer textIndex.Close()

	store, err := OpenSymbolStore(symbolDBPath(paths))
	if err != nil {
		return err
	}
	defer store.Close()

	if err := store.InitSchema(false); err != nil {
		return err
	}

	vectorCfg, vectorEnabled, err := loadVectorConfigForIndex()
	if err != nil {
		return err
	}

	collector := newIndexErrorCollector()
	var storeMu sync.Mutex
	var vectorRuntime *VectorRuntime
	if vectorEnabled {
		runtime, err := NewVectorRuntime(vectorCfg)
		if err != nil {
			collector.Add("vector_runtime", err)
			vectorEnabled = false
		} else {
			vectorRuntime = runtime
			defer vectorRuntime.Close()
			ctx := context.Background()
			if err := vectorRuntime.EnsureCollection(ctx); err != nil {
				collector.Add("vector_runtime", err)
				vectorEnabled = false
			} else if reporter != nil {
				mode := vectorStoreMode(vectorRuntime)
				announcePhase(reporter, fmt.Sprintf("phase:vector mode=%s batch=%d", mode, vectorCfg.VectorBatchSize))
			}
		}
	}

	var pkgIndex *GoPackageIndex
	if tierAllowsGoList(tier) {
		pkgIndex, err = BuildGoPackageIndex(paths.Root)
		if err != nil {
			collector.Add("go_list", err)
			pkgIndex = nil
		} else {
			if err := store.DeleteRelationsByKind(RelationKindDependsOn); err != nil {
				collector.Add("relations", err)
			}
			for _, rel := range pkgIndex.Depends {
				if err := store.InsertRelation(rel); err != nil {
					collector.Add("relations", err)
					break
				}
			}
		}
	}

	indexCtx := IndexContext{Tier: tier}
	if pkgIndex != nil {
		indexCtx.DirImportPath = pkgIndex.DirToImportPath
	}

	if reporter != nil {
		announcePhase(reporter, fmt.Sprintf("phase:delta files=%d", len(changes)))
		reporter.Start(len(changes))
		defer reporter.Finish()
	}

	for _, change := range changes {
		if change.OldPath != "" && shouldIndex(change.OldPath) {
			if err := removeFileIndex(store, textIndex, change.OldPath); err != nil {
				collector.Add(change.OldPath, err)
			}
		}

		switch change.Status {
		case "D":
			if shouldIndex(change.Path) {
				if err := removeFileIndex(store, textIndex, change.Path); err != nil {
					collector.Add(change.Path, err)
				}
				if vectorEnabled {
					if err := deleteVectorByFile(paths.Root, store, change.Path, vectorRuntime, &storeMu); err != nil {
						collector.Add("vector:"+change.Path, err)
					}
				}
			}
		default:
			if !shouldIndex(change.Path) {
				if reporter != nil {
					reporter.Increment()
				}
				continue
			}
			if err := removeFileIndex(store, textIndex, change.Path); err != nil {
				collector.Add(change.Path, err)
			}
			if vectorEnabled {
				if err := deleteVectorByFile(paths.Root, store, change.Path, vectorRuntime, &storeMu); err != nil {
					collector.Add("vector:"+change.Path, err)
				}
			}
			abs := filepath.Join(paths.Root, filepath.FromSlash(change.Path))
			content, err := os.ReadFile(abs)
			if err != nil {
				collector.Add(change.Path, err)
				if reporter != nil {
					reporter.Increment()
				}
				continue
			}
			ext := strings.ToLower(filepath.Ext(change.Path))
			switch ext {
			case ".go":
				if err := indexGoFile(store, bleveIndexAdapter{index: textIndex}, change.Path, content, &indexCtx); err != nil {
					collector.Add(change.Path, err)
					if reporter != nil {
						reporter.Increment()
					}
					continue
				}
				if vectorEnabled {
					if err := upsertVectorChunks(paths.Root, store, change.Path, content, nil, vectorRuntime, &storeMu); err != nil {
						collector.Add("vector:"+change.Path, err)
					}
				}
			case ".md", ".markdown":
				chunks := ChunkMarkdown(content)
				if len(chunks) == 0 {
					doc := TextDoc{
						Path:      change.Path,
						Kind:      "markdown",
						Content:   string(content),
						LineStart: 1,
						LineEnd:   lineCount(content),
					}
					docID := "md:" + change.Path + ":1"
					if err := textIndex.Index(docID, doc); err != nil {
						collector.Add(change.Path, err)
						if reporter != nil {
							reporter.Increment()
						}
						continue
					}
					if err := store.InsertTextDoc(change.Path, docID); err != nil {
						collector.Add(change.Path, err)
						if reporter != nil {
							reporter.Increment()
						}
						continue
					}
					break
				}
				for _, chunk := range chunks {
					docID := fmt.Sprintf("md:%s:%d", change.Path, chunk.LineStart)
					doc := TextDoc{
						Path:      change.Path,
						Kind:      "markdown",
						Title:     chunk.Title,
						Content:   chunk.Content,
						LineStart: chunk.LineStart,
						LineEnd:   chunk.LineEnd,
					}
					if err := textIndex.Index(docID, doc); err != nil {
						collector.Add(change.Path, err)
						break
					}
					if err := store.InsertTextDoc(change.Path, docID); err != nil {
						collector.Add(change.Path, err)
						break
					}
				}
				if vectorEnabled {
					if err := upsertVectorChunks(paths.Root, store, change.Path, content, chunks, vectorRuntime, &storeMu); err != nil {
						collector.Add("vector:"+change.Path, err)
					}
				}
			}
			if err := store.InsertFile(FileEntryFromContent(change.Path, ext, content)); err != nil {
				collector.Add(change.Path, err)
				if reporter != nil {
					reporter.Increment()
				}
				continue
			}
		}
		if reporter != nil {
			reporter.Increment()
		}
	}

	meta.LastIndexAt = time.Now()
	meta.UpdatedAt = time.Now()
	if err := SaveRepoMeta(paths, meta); err != nil {
		return err
	}
	return collector.Err()
}

func IndexRepoDeltaFromGit(root, rev string, reporter ProgressReporter) error {
	return IndexRepoDeltaFromGitWithOptions(root, rev, reporter, IndexOptions{})
}

func IndexRepoDeltaFromGitWithOptions(root, rev string, reporter ProgressReporter, opts IndexOptions) error {
	changes, err := gitDiffChanges(root, rev)
	if err != nil {
		return err
	}
	if len(changes) == 0 {
		if reporter != nil {
			reporter.Start(0)
			reporter.Finish()
		}
		return nil
	}
	return IndexRepoDeltaWithOptions(root, changes, reporter, opts)
}

func removeFileIndex(store *SymbolStore, textIndex bleve.Index, rel string) error {
	docIDs, err := store.ListTextDocIDs(rel)
	if err != nil {
		return err
	}
	if len(docIDs) == 0 {
		docIDs, err = findDocIDsByPath(textIndex, rel)
		if err != nil {
			return err
		}
	}
	for _, docID := range docIDs {
		_ = textIndex.Delete(docID)
	}
	if err := store.DeleteTextDocs(rel); err != nil {
		return err
	}
	if err := store.DeleteSymbolsByFile(rel); err != nil {
		return err
	}
	if err := store.DeleteRelationsByFile(rel); err != nil {
		return err
	}
	if err := store.DeleteDocLinksByFile(rel); err != nil {
		return err
	}
	if err := store.DeleteFile(rel); err != nil {
		return err
	}
	return nil
}

func upsertVectorChunks(root string, store *SymbolStore, rel string, content []byte, mdChunks []MDChunk, runtime *VectorRuntime, storeMu *sync.Mutex) error {
	if runtime == nil {
		return nil
	}
	ctx := context.Background()

	maxChars := runtime.cfg.VectorMaxChars
	var chunks []VectorChunk
	if mdChunks != nil {
		chunks = BuildMarkdownVectorChunks(rel, mdChunks, maxChars)
	} else {
		chunks = BuildGoVectorChunks(rel, content, maxChars, runtime.cfg.VectorOverlap)
	}
	if len(chunks) == 0 {
		return nil
	}

	texts := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		texts = append(texts, chunk.Text)
	}
	vectorMap := make(map[int][]float32, len(texts))
	batchSize := runtime.cfg.VectorBatchSize
	if batchSize <= 0 {
		batchSize = 8
	}
	for start := 0; start < len(texts); start += batchSize {
		end := start + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		vectors, err := runtime.embedder.EmbedTexts(ctx, texts[start:end])
		if err != nil {
			return err
		}
		for _, v := range vectors {
			vectorMap[start+v.Index] = v.Vector
		}
	}

	points := make([]VectorPoint, 0, len(chunks))
	vectorIDs := make([]string, 0, len(chunks))
	for i, chunk := range chunks {
		vec, ok := vectorMap[i]
		if !ok {
			continue
		}
		payload := map[string]any{
			"repo_id":    storeRepoID(root),
			"path":       chunk.File,
			"kind":       chunk.Kind,
			"name":       chunk.Name,
			"title":      chunk.Title,
			"line_start": chunk.LineStart,
			"line_end":   chunk.LineEnd,
			"hash":       chunk.Hash,
			"updated_at": time.Now().Unix(),
		}
		points = append(points, VectorPoint{
			ID:      chunk.ID,
			Vector:  vec,
			Payload: payload,
		})
		vectorIDs = append(vectorIDs, chunk.ID)
	}
	if err := runtime.store.UpsertPoints(ctx, runtime.cfg.QdrantCollection, points); err != nil {
		return err
	}
	if storeMu != nil {
		storeMu.Lock()
		defer storeMu.Unlock()
	}
	for _, id := range vectorIDs {
		if err := store.InsertVectorDoc(rel, id); err != nil {
			return err
		}
	}
	return nil
}

func deleteVectorByFile(root string, store *SymbolStore, rel string, runtime *VectorRuntime, storeMu *sync.Mutex) error {
	if runtime == nil {
		return nil
	}
	ctx := context.Background()
	if storeMu != nil {
		storeMu.Lock()
	}
	ids, err := store.ListVectorIDs(rel)
	if storeMu != nil {
		storeMu.Unlock()
	}
	if err != nil {
		return err
	}
	if len(ids) > 0 {
		if err := runtime.store.DeletePointsByIDs(ctx, runtime.cfg.QdrantCollection, ids); err != nil {
			return err
		}
	} else {
		if err := runtime.store.DeletePointsByRepoAndPath(ctx, runtime.cfg.QdrantCollection, storeRepoID(root), rel); err != nil {
			return err
		}
	}
	if storeMu != nil {
		storeMu.Lock()
		defer storeMu.Unlock()
	}
	return store.DeleteVectorDocs(rel)
}

func storeRepoID(root string) string {
	return repoID(root)
}

func announcePhase(reporter ProgressReporter, msg string) {
	if reporter == nil {
		return
	}
	fmt.Fprintln(os.Stderr, msg)
}

func vectorStoreMode(runtime *VectorRuntime) string {
	if runtime == nil || runtime.store == nil {
		return "unknown"
	}
	switch runtime.store.(type) {
	case *LocalVectorStore:
		return "local"
	case *QdrantStore:
		return "qdrant"
	default:
		return "unknown"
	}
}

func loadVectorConfigForIndex() (VectorConfig, bool, error) {
	cfg, ok, err := LoadVectorConfigOptional()
	if err != nil {
		return VectorConfig{}, false, err
	}
	if !ok {
		path, err := WriteDefaultVectorConfig()
		if err != nil {
			return VectorConfig{}, false, err
		}
		fmt.Fprintf(os.Stderr, "vector config not found; created default config at %s\n", path)
		cfg, err = LoadVectorConfig()
		if err != nil {
			return VectorConfig{}, false, err
		}
	}
	if !cfg.VectorEnabled {
		return cfg, false, nil
	}
	if strings.TrimSpace(cfg.VolcesAPIKey) == "" || strings.TrimSpace(cfg.VolcesModel) == "" {
		fmt.Fprintln(os.Stderr, "vector config missing volces_api_key or volces_model; vector indexing disabled")
		return cfg, false, nil
	}
	return cfg, true, nil
}

func findDocIDsByPath(textIndex bleve.Index, path string) ([]string, error) {
	query := bleve.NewMatchQuery(path)
	query.SetField("path")
	req := bleve.NewSearchRequestOptions(query, 1000, 0, false)
	req.Fields = []string{"path"}
	res, err := textIndex.Search(req)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, hit := range res.Hits {
		if val, ok := hit.Fields["path"].(string); ok && val == path {
			ids = append(ids, hit.ID)
		}
	}
	return ids, nil
}

func indexGoFile(store *SymbolStore, textIndex TextIndexer, rel string, content []byte, ctx *IndexContext) error {
	symbols, imports, err := ExtractGoSymbolsAndImports(rel, content)
	if err != nil {
		return fmt.Errorf("parse go file %s: %w", rel, err)
	}
	for _, sym := range symbols {
		if err := store.InsertSymbol(sym); err != nil {
			return err
		}
	}
	if err := insertImportRelations(store, rel, imports, ctx); err != nil {
		return err
	}

	return indexGoTextDocs(store, textIndex, rel, content)
}

type bleveIndexAdapter struct {
	index bleve.Index
}

func (b bleveIndexAdapter) IndexDoc(id string, doc TextDoc) error {
	return b.index.Index(id, doc)
}

func (b bleveIndexAdapter) Close() error {
	return nil
}

func indexGoTextDocs(store *SymbolStore, textIndex TextIndexer, rel string, content []byte) error {
	chunks := BuildGoVectorChunks(rel, content, 0, 0)
	if len(chunks) == 0 {
		doc := TextDoc{
			Path:      rel,
			Kind:      "file",
			Content:   string(content),
			LineStart: 1,
			LineEnd:   lineCount(content),
		}
		docID := "file:" + rel
		if err := textIndex.IndexDoc(docID, doc); err != nil {
			return err
		}
		return store.InsertTextDoc(rel, docID)
	}
	for _, chunk := range chunks {
		docID := fmt.Sprintf("go:%s:%d", rel, chunk.LineStart)
		doc := TextDoc{
			Path:      rel,
			Kind:      chunk.Kind,
			Title:     chunk.Name,
			Content:   chunk.Text,
			LineStart: chunk.LineStart,
			LineEnd:   chunk.LineEnd,
		}
		if err := textIndex.IndexDoc(docID, doc); err != nil {
			return err
		}
		if err := store.InsertTextDoc(rel, docID); err != nil {
			return err
		}
	}
	return nil
}

func insertImportRelations(store *SymbolStore, rel string, imports []GoImport, ctx *IndexContext) error {
	if len(imports) == 0 {
		return nil
	}
	fromRef := packageRefForFile(rel, ctx)
	for _, imp := range imports {
		relEntry := Relation{
			FromRef:    fromRef,
			ToRef:      imp.Path,
			Kind:       RelationKindImports,
			File:       rel,
			Line:       imp.Line,
			Source:     RelationSourceAST,
			Confidence: 1.0,
		}
		if err := store.InsertRelation(relEntry); err != nil {
			return err
		}
	}
	return nil
}

func packageRefForFile(rel string, ctx *IndexContext) string {
	dir := filepath.Dir(filepath.FromSlash(rel))
	dir = filepath.ToSlash(dir)
	if dir == "" || dir == "." {
		dir = "."
	}
	if ctx != nil && ctx.DirImportPath != nil {
		if imp, ok := ctx.DirImportPath[dir]; ok && strings.TrimSpace(imp) != "" {
			return imp
		}
	}
	return dir
}

func indexMarkdownFile(store *SymbolStore, textIndex TextIndexer, rel string, content []byte) error {
	chunks := ChunkMarkdown(content)
	links := ExtractMarkdownDocLinks(content)
	if len(chunks) == 0 {
		doc := TextDoc{
			Path:      rel,
			Kind:      "markdown",
			Content:   string(content),
			LineStart: 1,
			LineEnd:   lineCount(content),
		}
		docID := "md:" + rel + ":1"
		if err := textIndex.IndexDoc(docID, doc); err != nil {
			return err
		}
		if err := store.InsertTextDoc(rel, docID); err != nil {
			return err
		}
		return insertDocLinks(store, rel, links)
	}
	for _, chunk := range chunks {
		docID := fmt.Sprintf("md:%s:%d", rel, chunk.LineStart)
		doc := TextDoc{
			Path:      rel,
			Kind:      "markdown",
			Title:     chunk.Title,
			Content:   chunk.Content,
			LineStart: chunk.LineStart,
			LineEnd:   chunk.LineEnd,
		}
		if err := textIndex.IndexDoc(docID, doc); err != nil {
			return err
		}
		if err := store.InsertTextDoc(rel, docID); err != nil {
			return err
		}
	}
	return insertDocLinks(store, rel, links)
}

func lineCount(content []byte) int {
	if len(content) == 0 {
		return 0
	}
	return strings.Count(string(content), "\n") + 1
}

func insertDocLinks(store *SymbolStore, rel string, links []DocLink) error {
	if len(links) == 0 {
		return nil
	}
	for _, link := range links {
		if err := store.InsertDocLink(link, rel); err != nil {
			return err
		}
	}
	return nil
}

type FileEntry struct {
	Path  string
	Hash  string
	Lang  string
	Size  int64
	Mtime int64
}

func FileEntryFromContent(rel string, ext string, content []byte) FileEntry {
	hash := sha1.Sum(content)
	lang := "text"
	switch ext {
	case ".go":
		lang = "go"
	case ".md", ".markdown":
		lang = "markdown"
	}
	return FileEntry{
		Path:  rel,
		Hash:  hex.EncodeToString(hash[:]),
		Lang:  lang,
		Size:  int64(len(content)),
		Mtime: time.Now().Unix(),
	}
}
