package bcindex

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	count   int
	samples []string
}

func newIndexErrorCollector() *indexErrorCollector {
	return &indexErrorCollector{}
}

func (c *indexErrorCollector) Add(path string, err error) {
	if err == nil {
		return
	}
	c.count++
	if len(c.samples) < 5 {
		c.samples = append(c.samples, fmt.Sprintf("%s: %v", path, err))
	}
}

func (c *indexErrorCollector) Err() error {
	if c.count == 0 {
		return nil
	}
	return &IndexWarning{Count: c.count, Samples: c.samples}
}

func IndexRepo(root string) error {
	return IndexRepoWithProgress(root, nil)
}

func IndexRepoWithProgress(root string, reporter ProgressReporter) error {
	paths, meta, err := InitRepo(root)
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
		reporter.Start(len(indexable))
		defer reporter.Finish()
	}

	collector := newIndexErrorCollector()
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
			if err := indexGoFile(store, textIndex, rel, content); err != nil {
				collector.Add(rel, err)
				if reporter != nil {
					reporter.Increment()
				}
				continue
			}
		case ".md", ".markdown":
			if err := indexMarkdownFile(store, textIndex, rel, content); err != nil {
				collector.Add(rel, err)
				if reporter != nil {
					reporter.Increment()
				}
				continue
			}
		}
		if err := store.InsertFile(FileEntryFromContent(rel, ext, content)); err != nil {
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

	meta.LastIndexAt = time.Now()
	meta.UpdatedAt = time.Now()
	if err := SaveRepoMeta(paths, meta); err != nil {
		return err
	}
	return collector.Err()
}

func IndexRepoDelta(root string, changes []FileChange, reporter ProgressReporter) error {
	paths, meta, err := InitRepo(root)
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

	if reporter != nil {
		reporter.Start(len(changes))
		defer reporter.Finish()
	}

	collector := newIndexErrorCollector()
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
				doc := TextDoc{
					Path:      change.Path,
					Kind:      "file",
					Content:   string(content),
					LineStart: 1,
					LineEnd:   lineCount(content),
				}
				if err := textIndex.Index("file:"+change.Path, doc); err != nil {
					collector.Add(change.Path, err)
					if reporter != nil {
						reporter.Increment()
					}
					continue
				}
				if err := store.InsertTextDoc(change.Path, "file:"+change.Path); err != nil {
					collector.Add(change.Path, err)
					if reporter != nil {
						reporter.Increment()
					}
					continue
				}
				symbols, err := ExtractGoSymbols(change.Path, content)
				if err != nil {
					collector.Add(change.Path, err)
					if reporter != nil {
						reporter.Increment()
					}
					continue
				}
				for _, sym := range symbols {
					if err := store.InsertSymbol(sym); err != nil {
						collector.Add(change.Path, err)
						break
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
	return IndexRepoDelta(root, changes, reporter)
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
	if err := store.DeleteFile(rel); err != nil {
		return err
	}
	return nil
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

func indexGoFile(store *SymbolStore, textIndex TextIndexer, rel string, content []byte) error {
	symbols, err := ExtractGoSymbols(rel, content)
	if err != nil {
		return fmt.Errorf("parse go file %s: %w", rel, err)
	}
	for _, sym := range symbols {
		if err := store.InsertSymbol(sym); err != nil {
			return err
		}
	}

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
	if err := store.InsertTextDoc(rel, docID); err != nil {
		return err
	}
	return nil
}

func indexMarkdownFile(store *SymbolStore, textIndex TextIndexer, rel string, content []byte) error {
	chunks := ChunkMarkdown(content)
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
		return store.InsertTextDoc(rel, docID)
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
	return nil
}

func lineCount(content []byte) int {
	if len(content) == 0 {
		return 0
	}
	return strings.Count(string(content), "\n") + 1
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
