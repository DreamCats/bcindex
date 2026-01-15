package bcindex

func RepoStatus(paths RepoPaths, meta *RepoMeta) (Status, error) {
	if err := ensureIndex(paths, "mixed"); err != nil {
		return Status{}, err
	}
	store, err := OpenSymbolStore(symbolDBPath(paths))
	if err != nil {
		return Status{}, err
	}
	defer store.Close()

	if err := store.InitSchema(false); err != nil {
		return Status{}, err
	}

	symbolCount, err := store.CountSymbols()
	if err != nil {
		return Status{}, err
	}

	textIndex, err := OpenTextIndex(paths.TextDir)
	if err != nil {
		return Status{}, err
	}
	defer textIndex.Close()

	docCount, err := textIndex.DocCount()
	if err != nil {
		return Status{}, err
	}

	return Status{
		RepoID:      meta.RepoID,
		Root:        meta.Root,
		LastIndexAt: meta.LastIndexAt,
		Symbols:     symbolCount,
		TextDocs:    docCount,
	}, nil
}
