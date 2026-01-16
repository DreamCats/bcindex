package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// PackageStore provides CRUD operations for packages
type PackageStore struct {
	db *DB
}

// NewPackageStore creates a new package store
func NewPackageStore(db *DB) *PackageStore {
	return &PackageStore{db: db}
}

// Create inserts a new package
func (p *PackageStore) Create(pkg *Package) error {
	now := time.Now().UTC()
	pkg.CreatedAt = now
	pkg.UpdatedAt = now

	// Serialize JSON fields
	keyTypesJSON, err := json.Marshal(pkg.KeyTypes)
	if err != nil {
		return fmt.Errorf("failed to marshal key_types: %w", err)
	}

	keyFuncsJSON, err := json.Marshal(pkg.KeyFuncs)
	if err != nil {
		return fmt.Errorf("failed to marshal key_funcs: %w", err)
	}

	interfacesJSON, err := json.Marshal(pkg.Interfaces)
	if err != nil {
		return fmt.Errorf("failed to marshal interfaces: %w", err)
	}

	importsJSON, err := json.Marshal(pkg.Imports)
	if err != nil {
		return fmt.Errorf("failed to marshal imports: %w", err)
	}

	importedByJSON, err := json.Marshal(pkg.ImportedBy)
	if err != nil {
		return fmt.Errorf("failed to marshal imported_by: %w", err)
	}

	query := `
		INSERT INTO packages (
			path, name, role, summary, key_types, key_funcs, interfaces,
			imports, imported_by, file_count, symbol_count, line_count,
			repo_path, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			name = excluded.name,
			role = excluded.role,
			summary = excluded.summary,
			key_types = excluded.key_types,
			key_funcs = excluded.key_funcs,
			interfaces = excluded.interfaces,
			imports = excluded.imports,
			imported_by = COALESCE(excluded.imported_by, packages.imported_by),
			file_count = excluded.file_count,
			symbol_count = excluded.symbol_count,
			line_count = excluded.line_count,
			updated_at = excluded.updated_at
	`

	_, err = p.db.sqlDB.Exec(query,
		pkg.Path, pkg.Name, pkg.Role, pkg.Summary,
		string(keyTypesJSON), string(keyFuncsJSON), string(interfacesJSON),
		string(importsJSON), string(importedByJSON),
		pkg.FileCount, pkg.SymbolCount, pkg.LineCount,
		pkg.RepoPath, pkg.CreatedAt, pkg.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to insert package: %w", err)
	}

	return nil
}

// Get retrieves a package by path
func (p *PackageStore) Get(path string) (*Package, error) {
	query := `
		SELECT path, name, role, summary, key_types, key_funcs, interfaces,
			imports, imported_by, file_count, symbol_count, line_count,
			repo_path, created_at, updated_at
		FROM packages WHERE path = ?
	`

	row := p.db.sqlDB.QueryRow(query, path)
	pkg, err := p.scanPackageRow(row)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get package: %w", err)
	}

	return pkg, nil
}

// GetByRepo retrieves all packages in a repository
func (p *PackageStore) GetByRepo(repoPath string) ([]*Package, error) {
	query := `
		SELECT path, name, role, summary, key_types, key_funcs, interfaces,
			imports, imported_by, file_count, symbol_count, line_count,
			repo_path, created_at, updated_at
		FROM packages WHERE repo_path = ?
		ORDER BY path
	`

	rows, err := p.db.sqlDB.Query(query, repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to query packages: %w", err)
	}
	defer rows.Close()

	var packages []*Package
	for rows.Next() {
		pkg, err := p.scanPackageRow(rows)
		if err != nil {
			return nil, err
		}
		packages = append(packages, pkg)
	}

	return packages, nil
}

// Update updates a package
func (p *PackageStore) Update(pkg *Package) error {
	pkg.UpdatedAt = time.Now().UTC()

	keyTypesJSON, err := json.Marshal(pkg.KeyTypes)
	if err != nil {
		return fmt.Errorf("failed to marshal key_types: %w", err)
	}

	keyFuncsJSON, err := json.Marshal(pkg.KeyFuncs)
	if err != nil {
		return fmt.Errorf("failed to marshal key_funcs: %w", err)
	}

	interfacesJSON, err := json.Marshal(pkg.Interfaces)
	if err != nil {
		return fmt.Errorf("failed to marshal interfaces: %w", err)
	}

	importsJSON, err := json.Marshal(pkg.Imports)
	if err != nil {
		return fmt.Errorf("failed to marshal imports: %w", err)
	}

	importedByJSON, err := json.Marshal(pkg.ImportedBy)
	if err != nil {
		return fmt.Errorf("failed to marshal imported_by: %w", err)
	}

	query := `
		UPDATE packages SET
			name = ?, role = ?, summary = ?,
			key_types = ?, key_funcs = ?, interfaces = ?,
			imports = ?, imported_by = ?,
			file_count = ?, symbol_count = ?, line_count = ?,
			updated_at = ?
		WHERE path = ?
	`

	_, err = p.db.sqlDB.Exec(query,
		pkg.Name, pkg.Role, pkg.Summary,
		string(keyTypesJSON), string(keyFuncsJSON), string(interfacesJSON),
		string(importsJSON), string(importedByJSON),
		pkg.FileCount, pkg.SymbolCount, pkg.LineCount,
		pkg.UpdatedAt, pkg.Path,
	)

	if err != nil {
		return fmt.Errorf("failed to update package: %w", err)
	}

	return nil
}

// Delete removes a package
func (p *PackageStore) Delete(path string) error {
	_, err := p.db.sqlDB.Exec("DELETE FROM packages WHERE path = ?", path)
	if err != nil {
		return fmt.Errorf("failed to delete package: %w", err)
	}
	return nil
}

// DeleteByRepo removes all packages in a repository
func (p *PackageStore) DeleteByRepo(repoPath string) error {
	_, err := p.db.sqlDB.Exec("DELETE FROM packages WHERE repo_path = ?", repoPath)
	if err != nil {
		return fmt.Errorf("failed to delete packages: %w", err)
	}
	return nil
}

// AddImportedBy adds an import relationship to a package
func (p *PackageStore) AddImportedBy(pkgPath string, importerPath string) error {
	query := `
		UPDATE packages
		SET imported_by = json_array(
			SELECT DISTINCT value FROM json_each(imported_by)
			WHERE value != ? AND value IS NOT NULL
			UNION
			SELECT ?
		),
		updated_at = ?
		WHERE path = ?
	`

	now := time.Now().UTC()
	_, err := p.db.sqlDB.Exec(query, importerPath, importerPath, now, pkgPath)
	if err != nil {
		return fmt.Errorf("failed to add imported_by: %w", err)
	}
	return nil
}

// Count returns the number of packages
func (p *PackageStore) Count() (int, error) {
	var count int
	err := p.db.sqlDB.QueryRow("SELECT COUNT(*) FROM packages").Scan(&count)
	return count, err
}

// CountByRepo returns the number of packages in a repository
func (p *PackageStore) CountByRepo(repoPath string) (int, error) {
	var count int
	err := p.db.sqlDB.QueryRow("SELECT COUNT(*) FROM packages WHERE repo_path = ?", repoPath).Scan(&count)
	return count, err
}

// scanPackage scans a row into a Package
func (p *PackageStore) scanPackageRow(scanner rowScanner) (*Package, error) {
	pkg := &Package{}
	var keyTypesJSON, keyFuncsJSON, interfacesJSON, importsJSON, importedByJSON string
	var createdAtValue any
	var updatedAtValue any

	err := scanner.Scan(
		&pkg.Path, &pkg.Name, &pkg.Role, &pkg.Summary,
		&keyTypesJSON, &keyFuncsJSON, &interfacesJSON,
		&importsJSON, &importedByJSON,
		&pkg.FileCount, &pkg.SymbolCount, &pkg.LineCount,
		&pkg.RepoPath, &createdAtValue, &updatedAtValue,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to scan package: %w", err)
	}

	createdAt, err := parseTimeValue(createdAtValue)
	if err != nil {
		return nil, fmt.Errorf("failed to parse created_at: %w", err)
	}
	updatedAt, err := parseTimeValue(updatedAtValue)
	if err != nil {
		return nil, fmt.Errorf("failed to parse updated_at: %w", err)
	}
	pkg.CreatedAt = createdAt
	pkg.UpdatedAt = updatedAt

	if err := p.unmarshalJSONFields(keyTypesJSON, keyFuncsJSON, interfacesJSON, importsJSON, importedByJSON, pkg); err != nil {
		return nil, err
	}

	return pkg, nil
}

// unmarshalJSONFields unmarshals JSON fields into a Package
func (p *PackageStore) unmarshalJSONFields(keyTypesJSON, keyFuncsJSON, interfacesJSON, importsJSON, importedByJSON string, pkg *Package) error {
	if keyTypesJSON != "" {
		if err := json.Unmarshal([]byte(keyTypesJSON), &pkg.KeyTypes); err != nil {
			return fmt.Errorf("failed to unmarshal key_types: %w", err)
		}
	}

	if keyFuncsJSON != "" {
		if err := json.Unmarshal([]byte(keyFuncsJSON), &pkg.KeyFuncs); err != nil {
			return fmt.Errorf("failed to unmarshal key_funcs: %w", err)
		}
	}

	if interfacesJSON != "" {
		if err := json.Unmarshal([]byte(interfacesJSON), &pkg.Interfaces); err != nil {
			return fmt.Errorf("failed to unmarshal interfaces: %w", err)
		}
	}

	if importsJSON != "" {
		if err := json.Unmarshal([]byte(importsJSON), &pkg.Imports); err != nil {
			return fmt.Errorf("failed to unmarshal imports: %w", err)
		}
	}

	if importedByJSON != "" {
		if err := json.Unmarshal([]byte(importedByJSON), &pkg.ImportedBy); err != nil {
			return fmt.Errorf("failed to unmarshal imported_by: %w", err)
		}
	}

	return nil
}

// GetByRole retrieves packages by their inferred role
func (p *PackageStore) GetByRole(role string) ([]*Package, error) {
	query := `
		SELECT path, name, role, summary, key_types, key_funcs, interfaces,
			imports, imported_by, file_count, symbol_count, line_count,
			repo_path, created_at, updated_at
		FROM packages WHERE role = ?
		ORDER BY path
	`

	rows, err := p.db.sqlDB.Query(query, role)
	if err != nil {
		return nil, fmt.Errorf("failed to query packages by role: %w", err)
	}
	defer rows.Close()

	var packages []*Package
	for rows.Next() {
		pkg, err := p.scanPackageRow(rows)
		if err != nil {
			return nil, err
		}
		packages = append(packages, pkg)
	}

	return packages, nil
}
