package bcindex

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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

	if err := store.InitSchema(); err != nil {
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

	for _, rel := range indexable {
		abs := filepath.Join(paths.Root, filepath.FromSlash(rel))
		content, err := os.ReadFile(abs)
		if err != nil {
			return fmt.Errorf("read %s: %w", rel, err)
		}
		ext := strings.ToLower(filepath.Ext(rel))
		switch ext {
		case ".go":
			if err := indexGoFile(store, textIndex, rel, content); err != nil {
				return err
			}
		case ".md", ".markdown":
			if err := indexMarkdownFile(textIndex, rel, content); err != nil {
				return err
			}
		}
		if err := store.InsertFile(FileEntryFromContent(rel, ext, content)); err != nil {
			return err
		}
		if reporter != nil {
			reporter.Increment()
		}
	}

	meta.LastIndexAt = time.Now()
	meta.UpdatedAt = time.Now()
	return SaveRepoMeta(paths, meta)
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
	if err := textIndex.IndexDoc("file:"+rel, doc); err != nil {
		return err
	}
	return nil
}

func indexMarkdownFile(textIndex TextIndexer, rel string, content []byte) error {
	chunks := ChunkMarkdown(content)
	if len(chunks) == 0 {
		doc := TextDoc{
			Path:      rel,
			Kind:      "markdown",
			Content:   string(content),
			LineStart: 1,
			LineEnd:   lineCount(content),
		}
		return textIndex.IndexDoc("md:"+rel+":1", doc)
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
