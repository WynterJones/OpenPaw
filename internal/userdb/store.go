package userdb

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/database"
)

const (
	DefaultTableName  = "Table 1"
	DefaultColumnName = "Name"
	maxRowsPerQuery   = 500
)

var validColumnTypes = map[string]bool{
	"text":      true,
	"long_text": true,
	"number":    true,
	"checkbox":  true,
	"date":      true,
	"url":       true,
	"email":     true,
	"select":    true,
}

type Store struct {
	db *database.DB
}

func NewStore(db *database.DB) *Store {
	return &Store{db: db}
}

type Database struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	TableCount  int       `json:"table_count"`
	RowCount    int       `json:"row_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Tables      []Table   `json:"tables,omitempty"`
}

type Table struct {
	ID         string    `json:"id"`
	DatabaseID string    `json:"database_id"`
	Name       string    `json:"name"`
	SortOrder  int       `json:"sort_order"`
	RowCount   int       `json:"row_count"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	Columns    []Column  `json:"columns"`
}

type Column struct {
	ID        string                 `json:"id"`
	TableID   string                 `json:"table_id"`
	Name      string                 `json:"name"`
	Type      string                 `json:"type"`
	Options   map[string]interface{} `json:"options"`
	SortOrder int                    `json:"sort_order"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

type Row struct {
	ID        string                 `json:"id"`
	TableID   string                 `json:"table_id"`
	Values    map[string]interface{} `json:"values"`
	SortOrder int                    `json:"sort_order"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

type RowPage struct {
	Rows   []Row `json:"rows"`
	Total  int   `json:"total"`
	Limit  int   `json:"limit"`
	Offset int   `json:"offset"`
}

type NamedRows struct {
	Columns []string                 `json:"columns"`
	Rows    [][]interface{}          `json:"rows"`
	Records []map[string]interface{} `json:"records"`
	Total   int                      `json:"total"`
}

func cleanName(name string) string {
	return strings.TrimSpace(name)
}

func (s *Store) ListDatabases(workspaceID string) ([]Database, error) {
	rows, err := s.db.Query(`
		SELECT d.id, d.workspace_id, d.name, d.description, d.created_at, d.updated_at,
		       COUNT(DISTINCT t.id),
		       COUNT(r.id)
		  FROM user_databases d
		  LEFT JOIN user_database_tables t ON t.database_id = d.id
		  LEFT JOIN user_database_rows r ON r.table_id = t.id
		 WHERE d.workspace_id = ?
		 GROUP BY d.id
		 ORDER BY d.updated_at DESC, d.name COLLATE NOCASE`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Database{}
	for rows.Next() {
		var d Database
		if err := rows.Scan(&d.ID, &d.WorkspaceID, &d.Name, &d.Description, &d.CreatedAt, &d.UpdatedAt, &d.TableCount, &d.RowCount); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) CreateDatabase(workspaceID, name, description string) (Database, error) {
	name = cleanName(name)
	if name == "" {
		return Database{}, errors.New("name is required")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return Database{}, err
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	databaseID := uuid.New().String()
	tableID := uuid.New().String()
	columnID := uuid.New().String()
	if _, err := tx.Exec(
		`INSERT INTO user_databases (id, workspace_id, name, description, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		databaseID, workspaceID, name, strings.TrimSpace(description), now, now,
	); err != nil {
		return Database{}, friendlyConstraint(err, "a database with that name already exists")
	}
	if _, err := tx.Exec(
		`INSERT INTO user_database_tables (id, database_id, name, sort_order, created_at, updated_at)
		 VALUES (?, ?, ?, 0, ?, ?)`,
		tableID, databaseID, DefaultTableName, now, now,
	); err != nil {
		return Database{}, err
	}
	if _, err := tx.Exec(
		`INSERT INTO user_database_columns (id, table_id, name, type, options, sort_order, created_at, updated_at)
		 VALUES (?, ?, ?, 'text', '{}', 0, ?, ?)`,
		columnID, tableID, DefaultColumnName, now, now,
	); err != nil {
		return Database{}, err
	}
	if err := tx.Commit(); err != nil {
		return Database{}, err
	}
	return s.GetDatabase(workspaceID, databaseID)
}

func (s *Store) GetDatabase(workspaceID, id string) (Database, error) {
	var d Database
	err := s.db.QueryRow(`
		SELECT id, workspace_id, name, description, created_at, updated_at
		  FROM user_databases WHERE id = ? AND workspace_id = ?`, id, workspaceID,
	).Scan(&d.ID, &d.WorkspaceID, &d.Name, &d.Description, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return Database{}, err
	}
	tables, err := s.listTables(workspaceID, id)
	if err != nil {
		return Database{}, err
	}
	d.Tables = tables
	d.TableCount = len(tables)
	for _, t := range tables {
		d.RowCount += t.RowCount
	}
	return d, nil
}

func (s *Store) UpdateDatabase(workspaceID, id string, name, description *string) (Database, error) {
	var exists string
	if err := s.db.QueryRow("SELECT id FROM user_databases WHERE id = ? AND workspace_id = ?", id, workspaceID).Scan(&exists); err != nil {
		return Database{}, err
	}

	set := []string{"updated_at = ?"}
	args := []interface{}{time.Now().UTC()}
	if name != nil {
		value := cleanName(*name)
		if value == "" {
			return Database{}, errors.New("name cannot be empty")
		}
		set = append(set, "name = ?")
		args = append(args, value)
	}
	if description != nil {
		set = append(set, "description = ?")
		args = append(args, strings.TrimSpace(*description))
	}
	args = append(args, id, workspaceID)
	if _, err := s.db.Exec(
		"UPDATE user_databases SET "+strings.Join(set, ", ")+" WHERE id = ? AND workspace_id = ?",
		args...,
	); err != nil {
		return Database{}, friendlyConstraint(err, "a database with that name already exists")
	}
	return s.GetDatabase(workspaceID, id)
}

func (s *Store) DeleteDatabase(workspaceID, id string) error {
	result, err := s.db.Exec("DELETE FROM user_databases WHERE id = ? AND workspace_id = ?", id, workspaceID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) listTables(workspaceID, databaseID string) ([]Table, error) {
	rows, err := s.db.Query(`
		SELECT t.id, t.database_id, t.name, t.sort_order, t.created_at, t.updated_at,
		       COUNT(r.id)
		  FROM user_database_tables t
		  JOIN user_databases d ON d.id = t.database_id
		  LEFT JOIN user_database_rows r ON r.table_id = t.id
		 WHERE t.database_id = ? AND d.workspace_id = ?
		 GROUP BY t.id
		 ORDER BY t.sort_order, t.created_at`, databaseID, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tables := []Table{}
	for rows.Next() {
		var t Table
		if err := rows.Scan(&t.ID, &t.DatabaseID, &t.Name, &t.SortOrder, &t.CreatedAt, &t.UpdatedAt, &t.RowCount); err != nil {
			return nil, err
		}
		columns, err := s.ListColumns(workspaceID, t.ID)
		if err != nil {
			return nil, err
		}
		t.Columns = columns
		tables = append(tables, t)
	}
	return tables, rows.Err()
}

func (s *Store) CreateTable(workspaceID, databaseID, name string) (Table, error) {
	name = cleanName(name)
	if name == "" {
		return Table{}, errors.New("name is required")
	}
	if err := s.assertDatabase(workspaceID, databaseID); err != nil {
		return Table{}, err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return Table{}, err
	}
	defer tx.Rollback()
	var sortOrder int
	_ = tx.QueryRow("SELECT COALESCE(MAX(sort_order), -1) + 1 FROM user_database_tables WHERE database_id = ?", databaseID).Scan(&sortOrder)
	now := time.Now().UTC()
	tableID := uuid.New().String()
	if _, err := tx.Exec(
		`INSERT INTO user_database_tables (id, database_id, name, sort_order, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		tableID, databaseID, name, sortOrder, now, now,
	); err != nil {
		return Table{}, friendlyConstraint(err, "a table with that name already exists")
	}
	columnID := uuid.New().String()
	if _, err := tx.Exec(
		`INSERT INTO user_database_columns (id, table_id, name, type, options, sort_order, created_at, updated_at)
		 VALUES (?, ?, ?, 'text', '{}', 0, ?, ?)`,
		columnID, tableID, DefaultColumnName, now, now,
	); err != nil {
		return Table{}, err
	}
	if _, err := tx.Exec("UPDATE user_databases SET updated_at = ? WHERE id = ?", now, databaseID); err != nil {
		return Table{}, err
	}
	if err := tx.Commit(); err != nil {
		return Table{}, err
	}
	return s.GetTable(workspaceID, tableID)
}

func (s *Store) GetTable(workspaceID, tableID string) (Table, error) {
	var t Table
	err := s.db.QueryRow(`
		SELECT t.id, t.database_id, t.name, t.sort_order, t.created_at, t.updated_at,
		       (SELECT COUNT(*) FROM user_database_rows r WHERE r.table_id = t.id)
		  FROM user_database_tables t
		  JOIN user_databases d ON d.id = t.database_id
		 WHERE t.id = ? AND d.workspace_id = ?`, tableID, workspaceID,
	).Scan(&t.ID, &t.DatabaseID, &t.Name, &t.SortOrder, &t.CreatedAt, &t.UpdatedAt, &t.RowCount)
	if err != nil {
		return Table{}, err
	}
	t.Columns, err = s.ListColumns(workspaceID, tableID)
	return t, err
}

func (s *Store) UpdateTable(workspaceID, tableID string, name *string) (Table, error) {
	t, err := s.GetTable(workspaceID, tableID)
	if err != nil {
		return Table{}, err
	}
	if name != nil {
		value := cleanName(*name)
		if value == "" {
			return Table{}, errors.New("name cannot be empty")
		}
		now := time.Now().UTC()
		if _, err := s.db.Exec("UPDATE user_database_tables SET name = ?, updated_at = ? WHERE id = ?", value, now, tableID); err != nil {
			return Table{}, friendlyConstraint(err, "a table with that name already exists")
		}
		_, _ = s.db.Exec("UPDATE user_databases SET updated_at = ? WHERE id = ?", now, t.DatabaseID)
	}
	return s.GetTable(workspaceID, tableID)
}

func (s *Store) DeleteTable(workspaceID, tableID string) error {
	t, err := s.GetTable(workspaceID, tableID)
	if err != nil {
		return err
	}
	result, err := s.db.Exec("DELETE FROM user_database_tables WHERE id = ?", tableID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	_, _ = s.db.Exec("UPDATE user_databases SET updated_at = ? WHERE id = ?", time.Now().UTC(), t.DatabaseID)
	return nil
}

func (s *Store) ListColumns(workspaceID, tableID string) ([]Column, error) {
	if err := s.assertTable(workspaceID, tableID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`
		SELECT c.id, c.table_id, c.name, c.type, c.options, c.sort_order, c.created_at, c.updated_at
		  FROM user_database_columns c
		 WHERE c.table_id = ?
		 ORDER BY c.sort_order, c.created_at`, tableID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := []Column{}
	for rows.Next() {
		var c Column
		var options string
		if err := rows.Scan(&c.ID, &c.TableID, &c.Name, &c.Type, &options, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.Options = map[string]interface{}{}
		_ = json.Unmarshal([]byte(options), &c.Options)
		columns = append(columns, c)
	}
	return columns, rows.Err()
}

func (s *Store) CreateColumn(workspaceID, tableID, name, columnType string, options map[string]interface{}) (Column, error) {
	name = cleanName(name)
	if name == "" {
		return Column{}, errors.New("name is required")
	}
	if !validColumnTypes[columnType] {
		return Column{}, fmt.Errorf("unsupported column type %q", columnType)
	}
	t, err := s.GetTable(workspaceID, tableID)
	if err != nil {
		return Column{}, err
	}
	if options == nil {
		options = map[string]interface{}{}
	}
	optionsJSON, err := json.Marshal(options)
	if err != nil {
		return Column{}, errors.New("invalid column options")
	}
	var sortOrder int
	_ = s.db.QueryRow("SELECT COALESCE(MAX(sort_order), -1) + 1 FROM user_database_columns WHERE table_id = ?", tableID).Scan(&sortOrder)
	id := uuid.New().String()
	now := time.Now().UTC()
	if _, err := s.db.Exec(
		`INSERT INTO user_database_columns (id, table_id, name, type, options, sort_order, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, tableID, name, columnType, string(optionsJSON), sortOrder, now, now,
	); err != nil {
		return Column{}, friendlyConstraint(err, "a column with that name already exists")
	}
	s.touchTable(t)
	return s.getColumn(workspaceID, id)
}

func (s *Store) UpdateColumn(workspaceID, columnID string, name, columnType *string, options map[string]interface{}) (Column, error) {
	current, err := s.getColumn(workspaceID, columnID)
	if err != nil {
		return Column{}, err
	}
	set := []string{"updated_at = ?"}
	args := []interface{}{time.Now().UTC()}
	if name != nil {
		value := cleanName(*name)
		if value == "" {
			return Column{}, errors.New("name cannot be empty")
		}
		set = append(set, "name = ?")
		args = append(args, value)
	}
	if columnType != nil {
		if !validColumnTypes[*columnType] {
			return Column{}, fmt.Errorf("unsupported column type %q", *columnType)
		}
		set = append(set, "type = ?")
		args = append(args, *columnType)
	}
	if options != nil {
		value, err := json.Marshal(options)
		if err != nil {
			return Column{}, errors.New("invalid column options")
		}
		set = append(set, "options = ?")
		args = append(args, string(value))
	}
	args = append(args, columnID)
	if _, err := s.db.Exec("UPDATE user_database_columns SET "+strings.Join(set, ", ")+" WHERE id = ?", args...); err != nil {
		return Column{}, friendlyConstraint(err, "a column with that name already exists")
	}
	t, _ := s.GetTable(workspaceID, current.TableID)
	s.touchTable(t)
	return s.getColumn(workspaceID, columnID)
}

func (s *Store) DeleteColumn(workspaceID, columnID string) error {
	column, err := s.getColumn(workspaceID, columnID)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.Query("SELECT id, data FROM user_database_rows WHERE table_id = ?", column.TableID)
	if err != nil {
		return err
	}
	type rowData struct {
		id   string
		data map[string]interface{}
	}
	var updates []rowData
	for rows.Next() {
		var id, raw string
		if err := rows.Scan(&id, &raw); err != nil {
			rows.Close()
			return err
		}
		values := map[string]interface{}{}
		_ = json.Unmarshal([]byte(raw), &values)
		delete(values, columnID)
		updates = append(updates, rowData{id: id, data: values})
	}
	rows.Close()
	for _, update := range updates {
		raw, _ := json.Marshal(update.data)
		if _, err := tx.Exec("UPDATE user_database_rows SET data = ?, updated_at = ? WHERE id = ?", string(raw), time.Now().UTC(), update.id); err != nil {
			return err
		}
	}
	if _, err := tx.Exec("DELETE FROM user_database_columns WHERE id = ?", columnID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	t, _ := s.GetTable(workspaceID, column.TableID)
	s.touchTable(t)
	return nil
}

func (s *Store) ListRows(workspaceID, tableID, search string, limit, offset int) (RowPage, error) {
	if err := s.assertTable(workspaceID, tableID); err != nil {
		return RowPage{}, err
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > maxRowsPerQuery {
		limit = maxRowsPerQuery
	}
	if offset < 0 {
		offset = 0
	}
	where := "table_id = ?"
	args := []interface{}{tableID}
	if search = strings.TrimSpace(search); search != "" {
		where += " AND LOWER(data) LIKE ? ESCAPE '\\'"
		args = append(args, "%"+escapeLike(strings.ToLower(search))+"%")
	}

	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM user_database_rows WHERE "+where, args...).Scan(&total); err != nil {
		return RowPage{}, err
	}
	queryArgs := append(append([]interface{}{}, args...), limit, offset)
	rows, err := s.db.Query(
		`SELECT id, table_id, data, sort_order, created_at, updated_at
		   FROM user_database_rows WHERE `+where+`
		  ORDER BY sort_order, created_at LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return RowPage{}, err
	}
	defer rows.Close()

	out := []Row{}
	for rows.Next() {
		var row Row
		var raw string
		if err := rows.Scan(&row.ID, &row.TableID, &raw, &row.SortOrder, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return RowPage{}, err
		}
		row.Values = map[string]interface{}{}
		_ = json.Unmarshal([]byte(raw), &row.Values)
		out = append(out, row)
	}
	return RowPage{Rows: out, Total: total, Limit: limit, Offset: offset}, rows.Err()
}

func (s *Store) NamedRows(workspaceID, tableID, search string, limit, offset int) (NamedRows, error) {
	columns, err := s.ListColumns(workspaceID, tableID)
	if err != nil {
		return NamedRows{}, err
	}
	page, err := s.ListRows(workspaceID, tableID, search, limit, offset)
	if err != nil {
		return NamedRows{}, err
	}
	out := NamedRows{Columns: []string{}, Rows: [][]interface{}{}, Records: []map[string]interface{}{}, Total: page.Total}
	for _, column := range columns {
		out.Columns = append(out.Columns, column.Name)
	}
	for _, row := range page.Rows {
		values := make([]interface{}, 0, len(columns))
		record := map[string]interface{}{"_row_id": row.ID}
		for _, column := range columns {
			value := row.Values[column.ID]
			values = append(values, value)
			record[column.Name] = value
		}
		out.Rows = append(out.Rows, values)
		out.Records = append(out.Records, record)
	}
	return out, nil
}

func (s *Store) CreateRow(workspaceID, tableID string, values map[string]interface{}) (Row, error) {
	if err := s.assertTable(workspaceID, tableID); err != nil {
		return Row{}, err
	}
	clean, err := s.validateValues(workspaceID, tableID, values)
	if err != nil {
		return Row{}, err
	}
	raw, _ := json.Marshal(clean)
	var sortOrder int
	_ = s.db.QueryRow("SELECT COALESCE(MAX(sort_order), -1) + 1 FROM user_database_rows WHERE table_id = ?", tableID).Scan(&sortOrder)
	id := uuid.New().String()
	now := time.Now().UTC()
	if _, err := s.db.Exec(
		`INSERT INTO user_database_rows (id, table_id, data, sort_order, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, tableID, string(raw), sortOrder, now, now,
	); err != nil {
		return Row{}, err
	}
	t, _ := s.GetTable(workspaceID, tableID)
	s.touchTable(t)
	return Row{ID: id, TableID: tableID, Values: clean, SortOrder: sortOrder, CreatedAt: now, UpdatedAt: now}, nil
}

func (s *Store) UpdateRow(workspaceID, rowID string, values map[string]interface{}) (Row, error) {
	row, err := s.getRow(workspaceID, rowID)
	if err != nil {
		return Row{}, err
	}
	clean, err := s.validateValues(workspaceID, row.TableID, values)
	if err != nil {
		return Row{}, err
	}
	for key, value := range clean {
		if value == nil {
			delete(row.Values, key)
		} else {
			row.Values[key] = value
		}
	}
	raw, _ := json.Marshal(row.Values)
	row.UpdatedAt = time.Now().UTC()
	if _, err := s.db.Exec("UPDATE user_database_rows SET data = ?, updated_at = ? WHERE id = ?", string(raw), row.UpdatedAt, rowID); err != nil {
		return Row{}, err
	}
	t, _ := s.GetTable(workspaceID, row.TableID)
	s.touchTable(t)
	return row, nil
}

func (s *Store) DeleteRow(workspaceID, rowID string) error {
	row, err := s.getRow(workspaceID, rowID)
	if err != nil {
		return err
	}
	result, err := s.db.Exec("DELETE FROM user_database_rows WHERE id = ?", rowID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	t, _ := s.GetTable(workspaceID, row.TableID)
	s.touchTable(t)
	return nil
}

// ColumnIDsForNames converts an agent-friendly {column name: value} object to
// the stable UUID-keyed representation stored in each row.
func (s *Store) ColumnIDsForNames(workspaceID, tableID string, values map[string]interface{}) (map[string]interface{}, error) {
	columns, err := s.ListColumns(workspaceID, tableID)
	if err != nil {
		return nil, err
	}
	byName := map[string]string{}
	for _, column := range columns {
		byName[strings.ToLower(column.Name)] = column.ID
	}
	out := map[string]interface{}{}
	for name, value := range values {
		id := byName[strings.ToLower(strings.TrimSpace(name))]
		if id == "" {
			return nil, fmt.Errorf("unknown column %q", name)
		}
		out[id] = value
	}
	return out, nil
}

func (s *Store) getColumn(workspaceID, columnID string) (Column, error) {
	var c Column
	var options string
	err := s.db.QueryRow(`
		SELECT c.id, c.table_id, c.name, c.type, c.options, c.sort_order, c.created_at, c.updated_at
		  FROM user_database_columns c
		  JOIN user_database_tables t ON t.id = c.table_id
		  JOIN user_databases d ON d.id = t.database_id
		 WHERE c.id = ? AND d.workspace_id = ?`, columnID, workspaceID,
	).Scan(&c.ID, &c.TableID, &c.Name, &c.Type, &options, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return Column{}, err
	}
	c.Options = map[string]interface{}{}
	_ = json.Unmarshal([]byte(options), &c.Options)
	return c, nil
}

func (s *Store) getRow(workspaceID, rowID string) (Row, error) {
	var row Row
	var raw string
	err := s.db.QueryRow(`
		SELECT r.id, r.table_id, r.data, r.sort_order, r.created_at, r.updated_at
		  FROM user_database_rows r
		  JOIN user_database_tables t ON t.id = r.table_id
		  JOIN user_databases d ON d.id = t.database_id
		 WHERE r.id = ? AND d.workspace_id = ?`, rowID, workspaceID,
	).Scan(&row.ID, &row.TableID, &raw, &row.SortOrder, &row.CreatedAt, &row.UpdatedAt)
	if err != nil {
		return Row{}, err
	}
	row.Values = map[string]interface{}{}
	_ = json.Unmarshal([]byte(raw), &row.Values)
	return row, nil
}

func (s *Store) validateValues(workspaceID, tableID string, values map[string]interface{}) (map[string]interface{}, error) {
	columns, err := s.ListColumns(workspaceID, tableID)
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, column := range columns {
		known[column.ID] = true
	}
	clean := map[string]interface{}{}
	for id, value := range values {
		if !known[id] {
			return nil, fmt.Errorf("unknown column id %q", id)
		}
		clean[id] = value
	}
	return clean, nil
}

func (s *Store) assertDatabase(workspaceID, databaseID string) error {
	var id string
	return s.db.QueryRow("SELECT id FROM user_databases WHERE id = ? AND workspace_id = ?", databaseID, workspaceID).Scan(&id)
}

func (s *Store) assertTable(workspaceID, tableID string) error {
	var id string
	return s.db.QueryRow(`
		SELECT t.id FROM user_database_tables t
		JOIN user_databases d ON d.id = t.database_id
		WHERE t.id = ? AND d.workspace_id = ?`, tableID, workspaceID).Scan(&id)
}

func (s *Store) touchTable(t Table) {
	now := time.Now().UTC()
	_, _ = s.db.Exec("UPDATE user_database_tables SET updated_at = ? WHERE id = ?", now, t.ID)
	_, _ = s.db.Exec("UPDATE user_databases SET updated_at = ? WHERE id = ?", now, t.DatabaseID)
}

func friendlyConstraint(err error, message string) error {
	if strings.Contains(strings.ToLower(err.Error()), "unique constraint") {
		return errors.New(message)
	}
	return err
}

func escapeLike(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_")
	return replacer.Replace(value)
}
