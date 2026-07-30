package userdb

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const MaxCSVRows = 100000

type CSVImportResult struct {
	Database     Database `json:"database"`
	ImportedRows int      `json:"imported_rows"`
}

// ImportCSV creates a database whose first table mirrors the CSV headers and
// whose rows preserve the source cell values as text.
func (s *Store) ImportCSV(workspaceID, filename string, source io.Reader) (result CSVImportResult, err error) {
	reader := csv.NewReader(source)
	reader.FieldsPerRecord = -1
	reader.ReuseRecord = false

	records, err := reader.ReadAll()
	if err != nil {
		return result, fmt.Errorf("invalid CSV: %w", err)
	}
	if len(records) == 0 {
		return result, errors.New("CSV must include a header row")
	}
	if len(records)-1 > MaxCSVRows {
		return result, fmt.Errorf("CSV cannot contain more than %d data rows", MaxCSVRows)
	}

	headers := normalizeCSVHeaders(records[0])
	if len(headers) == 0 {
		return result, errors.New("CSV must include at least one column")
	}

	baseName := strings.TrimSpace(strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename)))
	if baseName == "" || baseName == "." {
		baseName = "Imported database"
	}
	databaseName, err := s.availableDatabaseName(workspaceID, baseName)
	if err != nil {
		return result, err
	}

	item, err := s.CreateDatabase(workspaceID, databaseName, "Imported from "+filepath.Base(filename))
	if err != nil {
		return result, err
	}
	defer func() {
		if err != nil {
			_ = s.DeleteDatabase(workspaceID, item.ID)
		}
	}()

	table := item.Tables[0]
	tableName := databaseName
	table, err = s.UpdateTable(workspaceID, table.ID, &tableName)
	if err != nil {
		return result, err
	}

	firstHeader := headers[0]
	firstColumn, err := s.UpdateColumn(workspaceID, table.Columns[0].ID, &firstHeader, nil, nil)
	if err != nil {
		return result, err
	}
	columns := []Column{firstColumn}
	for _, header := range headers[1:] {
		column, createErr := s.CreateColumn(workspaceID, table.ID, header, "text", nil)
		if createErr != nil {
			return result, createErr
		}
		columns = append(columns, column)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	statement, err := tx.Prepare(`
		INSERT INTO user_database_rows (id, table_id, data, sort_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return result, err
	}
	defer statement.Close()
	now := time.Now().UTC()
	for rowIndex, record := range records[1:] {
		values := make(map[string]interface{}, len(columns))
		for index, column := range columns {
			if index < len(record) && record[index] != "" {
				values[column.ID] = record[index]
			}
		}
		raw, marshalErr := json.Marshal(values)
		if marshalErr != nil {
			return result, marshalErr
		}
		if _, insertErr := statement.Exec(uuid.New().String(), table.ID, string(raw), rowIndex, now, now); insertErr != nil {
			return result, insertErr
		}
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}

	item, err = s.GetDatabase(workspaceID, item.ID)
	if err != nil {
		return result, err
	}
	return CSVImportResult{Database: item, ImportedRows: len(records) - 1}, nil
}

// ExportTableCSV writes a table in display column order and row order.
func (s *Store) ExportTableCSV(workspaceID, tableID string, destination io.Writer) (int, error) {
	table, err := s.GetTable(workspaceID, tableID)
	if err != nil {
		return 0, err
	}

	writer := csv.NewWriter(destination)
	headers := make([]string, len(table.Columns))
	for index, column := range table.Columns {
		headers[index] = column.Name
	}
	if err := writer.Write(headers); err != nil {
		return 0, err
	}

	exported := 0
	for {
		page, pageErr := s.ListRows(workspaceID, tableID, "", maxRowsPerQuery, exported)
		if pageErr != nil {
			return exported, pageErr
		}
		for _, row := range page.Rows {
			record := make([]string, len(table.Columns))
			for index, column := range table.Columns {
				record[index] = csvCellString(row.Values[column.ID])
			}
			if writeErr := writer.Write(record); writeErr != nil {
				return exported, writeErr
			}
			exported++
		}
		if len(page.Rows) == 0 || exported >= page.Total {
			break
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return exported, err
	}
	return exported, nil
}

func (s *Store) availableDatabaseName(workspaceID, base string) (string, error) {
	items, err := s.ListDatabases(workspaceID)
	if err != nil {
		return "", err
	}
	used := make(map[string]bool, len(items))
	for _, item := range items {
		used[strings.ToLower(item.Name)] = true
	}
	if !used[strings.ToLower(base)] {
		return base, nil
	}
	for suffix := 2; ; suffix++ {
		candidate := base + " " + strconv.Itoa(suffix)
		if !used[strings.ToLower(candidate)] {
			return candidate, nil
		}
	}
}

func normalizeCSVHeaders(source []string) []string {
	headers := make([]string, len(source))
	used := make(map[string]int, len(source))
	for index, header := range source {
		header = strings.TrimSpace(strings.TrimPrefix(header, "\ufeff"))
		if header == "" {
			header = fmt.Sprintf("Column %d", index+1)
		}
		base := header
		key := strings.ToLower(base)
		used[key]++
		if used[key] > 1 {
			header = fmt.Sprintf("%s %d", base, used[key])
			for used[strings.ToLower(header)] > 0 {
				used[key]++
				header = fmt.Sprintf("%s %d", base, used[key])
			}
		}
		used[strings.ToLower(header)] = 1
		headers[index] = header
	}
	return headers
}

func csvCellString(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32)
	case json.Number:
		return typed.String()
	default:
		encoded, err := json.Marshal(typed)
		if err == nil {
			return string(encoded)
		}
		return fmt.Sprint(typed)
	}
}
