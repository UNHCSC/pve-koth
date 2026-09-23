package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

type additiveColumn struct {
	name       string
	definition string
}

func migrateDatabase(path string) error {
	database, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("open database for migrations: %w", err)
	}
	defer database.Close()

	columns, exists, err := tableColumns(database, "Container")
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	additions := []additiveColumn{
		{name: "guest_kind", definition: `TEXT NOT NULL DEFAULT 'lxc'`},
		{name: "guest_os", definition: `TEXT NOT NULL DEFAULT 'linux'`},
		{name: "script_shell", definition: `TEXT NOT NULL DEFAULT 'bash'`},
		{name: "template_ref", definition: `TEXT NOT NULL DEFAULT ''`},
		{name: "template_vmid", definition: `INTEGER NOT NULL DEFAULT 0`},
	}

	for _, addition := range additions {
		if _, ok := columns[addition.name]; ok {
			continue
		}
		statement := fmt.Sprintf(`ALTER TABLE "Container" ADD COLUMN "%s" %s`, addition.name, addition.definition)
		if _, err = database.Exec(statement); err != nil {
			return fmt.Errorf("add Container.%s: %w", addition.name, err)
		}
	}

	return nil
}

func tableColumns(database *sql.DB, table string) (map[string]struct{}, bool, error) {
	rows, err := database.Query(fmt.Sprintf(`PRAGMA table_info("%s")`, table))
	if err != nil {
		return nil, false, fmt.Errorf("inspect %s schema: %w", table, err)
	}
	defer rows.Close()

	columns := make(map[string]struct{})
	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultV   any
			primary    int
		)
		if err = rows.Scan(&cid, &name, &columnType, &notNull, &defaultV, &primary); err != nil {
			return nil, false, fmt.Errorf("read %s schema: %w", table, err)
		}
		columns[name] = struct{}{}
	}
	if err = rows.Err(); err != nil {
		return nil, false, fmt.Errorf("read %s schema: %w", table, err)
	}

	return columns, len(columns) > 0, nil
}
