package pluto

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

func CleanupImages(ctx context.Context) error {
	db := PlutoInstance.DbPool
	schema := PlutoInstance.DbSchema
	dir := PlutoInstance.Config.PlutoImageDir

	// Load filenames from DB
	query := fmt.Sprintf("SELECT gen_file_name FROM %s.pluto_image", schema)
	rows, err := db.Query(ctx, query)
	if err != nil {
		return fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	validFiles := make(map[string]struct{})
	for rows.Next() {
		var name *string // Handle NULL safely
		if err := rows.Scan(&name); err != nil {
			return err
		}
		if name != nil && *name != "" {
			validFiles[*name] = struct{}{}
		}
	}

	if err := rows.Err(); err != nil {
		return err
	}

	// Walk directory
	deleteCount := 0
	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Skip directories
		if d.IsDir() {
			return nil
		}
		fileName := filepath.Base(path)

		// Delete if not in DB
		if _, exists := validFiles[fileName]; !exists {
			fmt.Printf("Deleting: %s\n", path)
			deleteCount++
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("failed to delete %s: %w", path, err)
			}
		}

		return nil
	})

	if err != nil {
		return err
	}

	fmt.Printf("Deleted: %d images\n", deleteCount)

	return nil
}
