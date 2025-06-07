package main

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "modernc.org/sqlite"
)

// ColumnInfo holds metadata for a database column
type ColumnInfo struct {
	Name string
	Type string // MariaDB data type string (e.g., "int", "varchar", "datetime")
}

// main function to orchestrate the migration
func main() {
	// --- Configuration ---
	mariaDBConnStr := "wallabag:wallabag@tcp(127.0.0.1:3306)/wallabag?charset=utf8mb4&parseTime=true" // REPLACE with your MariaDB details
	sqliteDBPath := "./wallabag.sqlite"                                                               // REPLACE with your SQLite path
	mariaDBDatabaseName := "wallabag"                                                                 // REPLACE with your MariaDB database name

	// --- Database Connections ---
	log.Println("Connecting to MariaDB...")
	mariaDB, err := sql.Open("mysql", mariaDBConnStr)
	if err != nil {
		log.Fatalf("Failed to connect to MariaDB: %v", err)
	}
	defer mariaDB.Close()

	err = mariaDB.Ping()
	if err != nil {
		log.Fatalf("Failed to ping MariaDB: %v", err)
	}
	log.Println("Successfully connected to MariaDB.")

	log.Println("Connecting to SQLite...")
	sqliteDB, err := sql.Open("sqlite", sqliteDBPath) // 'sqlite3' is the driver name for modernc.org/sqlite
	if err != nil {
		log.Fatalf("Failed to connect to SQLite: %v", err)
	}
	defer sqliteDB.Close()

	err = sqliteDB.Ping()
	if err != nil {
		log.Fatalf("Failed to ping SQLite: %v", err)
	}
	log.Println("Successfully connected to SQLite.")

	// --- Migration Process ---

	// Disable foreign key checks for faster and smoother import
	_, err = sqliteDB.Exec("PRAGMA foreign_keys = OFF;")
	if err != nil {
		log.Fatalf("Failed to disable SQLite foreign keys: %v", err)
	}
	log.Println("SQLite foreign key checks disabled for import.")

	// Get list of tables from MariaDB
	tables, err := getMariaDBTables(mariaDB, mariaDBDatabaseName)
	if err != nil {
		log.Fatalf("Failed to get tables from MariaDB: %v", err)
	}

	// Use a map for quick lookup and to track migrated status
	tablesToMigrate := make(map[string]bool)
	for _, table := range tables {
		tablesToMigrate[table] = false // Not yet migrated
	}

	for _, tableName := range tables {
		if tablesToMigrate[tableName] { // Already migrated
			continue
		}

		log.Printf("Migrating table '%s'...", tableName)
		err = migrateTable(mariaDB, sqliteDB, mariaDBDatabaseName, tableName)
		if err != nil {
			log.Fatalf("Error migrating table '%s': %v", tableName, err)
		}
		log.Printf("Successfully migrated table '%s'.", tableName)
		tablesToMigrate[tableName] = true
	}

	// Re-enable foreign key checks
	_, err = sqliteDB.Exec("PRAGMA foreign_keys = ON;")
	if err != nil {
		log.Fatalf("Failed to re-enable SQLite foreign keys: %v", err)
	}
	log.Println("SQLite foreign key checks re-enabled.")

	log.Println("Migration complete!")
}

// getMariaDBTables retrieves a list of table names from the given MariaDB database
func getMariaDBTables(db *sql.DB, dbName string) ([]string, error) {
	rows, err := db.Query("SELECT table_name FROM information_schema.tables WHERE table_schema = ?", dbName)
	if err != nil {
		return nil, fmt.Errorf("querying tables failed: %w", err)
	}
	defer rows.Close()

	var tables []string
	var tableName string
	for rows.Next() {
		if err := rows.Scan(&tableName); err != nil {
			return nil, fmt.Errorf("scanning table name failed: %w", err)
		}
		tables = append(tables, tableName)
	}
	return tables, nil
}

// getMariaDBColumnInfo retrieves column names and types for a given table
func getMariaDBColumnInfo(db *sql.DB, dbName, tableName string) ([]ColumnInfo, error) {
	rows, err := db.Query(`
        SELECT COLUMN_NAME, DATA_TYPE
        FROM INFORMATION_SCHEMA.COLUMNS
        WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
        ORDER BY ORDINAL_POSITION
    `, dbName, tableName)
	if err != nil {
		return nil, fmt.Errorf("querying column info for table %s failed: %w", tableName, err)
	}
	defer rows.Close()

	var columns []ColumnInfo
	var colName, colType string
	for rows.Next() {
		if err := rows.Scan(&colName, &colType); err != nil {
			return nil, fmt.Errorf("scanning column info for table %s failed: %w", tableName, err)
		}
		columns = append(columns, ColumnInfo{Name: colName, Type: colType})
	}
	return columns, nil
}

// migrateTable performs the generic migration for a single table
func migrateTable(mariaDB *sql.DB, sqliteDB *sql.DB, mariaDBDatabaseName, tableName string) error {
	truncateQuery := fmt.Sprintf("DELETE FROM %s", tableName)

	// Get column information to build dynamic queries and handle types
	columns, err := getMariaDBColumnInfo(mariaDB, mariaDBDatabaseName, tableName)
	if err != nil {
		return fmt.Errorf("could not get column info for %s: %w", tableName, err)
	}

	// Build SELECT query string
	columnNames := make([]string, len(columns))
	for i, col := range columns {
		columnNames[i] = col.Name
	}
	selectQuery := fmt.Sprintf("SELECT %s FROM %s", strings.Join(columnNames, ", "), tableName)

	// Build INSERT query string
	placeholders := make([]string, len(columns))
	for i := range columns {
		placeholders[i] = "?"
	}
	insertQuery := fmt.Sprintf("INSERT OR REPLACE INTO %s (%s) VALUES (%s)",
		tableName, strings.Join(columnNames, ", "), strings.Join(placeholders, ", "))

	// Begin a transaction for SQLite
	tx, err := sqliteDB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin SQLite transaction for %s: %w", tableName, err)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		} else if err != nil {
			tx.Rollback()
		} else {
			err = tx.Commit()
		}
	}()

	_, err = tx.Exec(truncateQuery)
	if err != nil {
		return fmt.Errorf("failed to truncate table %s: %w", tableName, err)
	}

	// Prepare the SQLite INSERT statement
	stmt, err := tx.Prepare(insertQuery)
	if err != nil {
		return fmt.Errorf("failed to prepare SQLite insert statement for %s: %w", tableName, err)
	}
	defer stmt.Close()

	// Query data from MariaDB
	rows, err := mariaDB.Query(selectQuery)
	if err != nil {
		return fmt.Errorf("failed to query MariaDB table %s: %w", tableName, err)
	}
	defer rows.Close()

	// Prepare slices for scanning and arguments
	scanDest := make([]interface{}, len(columns))
	colValues := make([]interface{}, len(columns))

	recordCount := 0
	for rows.Next() {
		recordCount++

		// Initialize scan destinations dynamically based on MariaDB column types
		for i, col := range columns {
			colValues[i], err = mapMariaDBTypeToGoType(col.Type)
			if err != nil {
				return fmt.Errorf("unsupported MariaDB type %s for column %s in table %s: %w", col.Type, col.Name, tableName, err)
			}
			scanDest[i] = &colValues[i]
		}

		// Scan data from MariaDB row
		if err := rows.Scan(scanDest...); err != nil {
			return fmt.Errorf("failed to scan MariaDB row in table %s (record %d): %w", tableName, recordCount, err)
		}

		// Prepare arguments for SQLite insert, converting types as needed
		insertArgs := make([]interface{}, len(columns))
		for i, col := range columns {
			insertArgs[i] = convertToGoToSQLite(colValues[i], col.Type)
		}

		// Execute INSERT into SQLite
		if _, err := stmt.Exec(insertArgs...); err != nil {
			return fmt.Errorf("failed to insert record %d into SQLite table %s: %w", recordCount, tableName, err)
		}

		if recordCount%1000 == 0 {
			log.Printf("Migrated %d records in table '%s'...", recordCount, tableName)
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error during row iteration for table %s: %w", tableName, err)
	}

	log.Printf("Finished migrating %d records in table '%s'.", recordCount, tableName)
	return nil
}

// mapMariaDBTypeToGoType maps a MariaDB data type string to an appropriate Go type for scanning
// This is crucial for handling NULL values and preparing for type conversions.
func mapMariaDBTypeToGoType(mariaDBType string) (interface{}, error) {
	switch strings.ToLower(mariaDBType) {
	case "int", "tinyint", "smallint", "mediumint", "bigint":
		return sql.NullInt64{}, nil
	case "float", "double", "decimal", "numeric":
		return sql.NullFloat64{}, nil
	case "varchar", "text", "tinytext", "mediumtext", "longtext", "char", "json":
		return sql.NullString{}, nil // JSON will be read as strings/bytes
	case "blob", "longblob", "mediumblob", "tinyblob":
		// BLOBs are read as []byte. SQLite also supports BLOB type.
		// If you intend to convert them to text (e.g., base64), you'd handle it in convertToGoToSQLite
		return []byte{}, nil
	case "datetime", "timestamp", "date":
		return sql.NullTime{}, nil
	case "boolean": // MariaDB's BOOLEAN is a TINYINT(1)
		return sql.NullBool{}, nil
	// Add more types as needed based on your MariaDB schema
	default:
		// Fallback for unknown types to string, or return an error if strict
		log.Printf("Warning: Unhandled MariaDB type '%s'. Attempting to scan as string.", mariaDBType)
		return sql.NullString{}, nil // Default to string for unknown types
	}
}

// convertToGoToSQLite performs final type conversion from Go's sql.NullX types to SQLite's TEXT/INTEGER/REAL
// Newlines are NOT stripped here as per user's request.
func convertToGoToSQLite(val interface{}, mariaDBType string) interface{} {
	if val == nil {
		return nil // Handle database NULLs
	}

	lowerMariaDBType := strings.ToLower(mariaDBType)

	switch v := val.(type) {
	case int64:
		return v
	case float64:
		return v
	case bool:
		if v {
			return 1
		} // Convert Go bool to SQLite INTEGER (0 or 1)
		return 0
	case time.Time:
		// Convert time to ISO 8601 string for SQLite TEXT column
		return v.Format(time.RFC3339)
	case string:
		// This case handles data that was already scanned as a Go string.
		// Apply string cleaning: TRIM whitespace and remove null bytes.
		cleanedString := strings.TrimSpace(v)
		cleanedString = strings.ReplaceAll(cleanedString, string(rune(0)), "") // Remove NULL bytes (CHAR(0))
		return cleanedString
	case []byte:
		// This case is crucial for data that the MariaDB driver returned as raw bytes.
		// This can happen for VARCHAR, TEXT, JSON, and BLOB columns.
		// We need to differentiate based on the *original MariaDB type* to know how to treat it.
		if strings.Contains(lowerMariaDBType, "text") || strings.Contains(lowerMariaDBType, "char") || strings.Contains(lowerMariaDBType, "json") || strings.Contains(lowerMariaDBType, "varchar") {
			// It's a text-like column, convert []byte to string and then clean it.
			cleanedString := string(v)
			cleanedString = strings.TrimSpace(cleanedString)
			cleanedString = strings.ReplaceAll(cleanedString, string(rune(0)), "") // Remove NULL bytes
			return cleanedString
		} else if strings.Contains(lowerMariaDBType, "blob") {
			// It's a true BLOB, return as is. SQLite handles []byte as BLOB.
			return v
		}
		// Fallback for unexpected []byte if it's not a known text/blob type, treat as raw BLOB.
		log.Printf("Warning: Unexpected []byte for MariaDB type '%s'. Treating as raw BLOB.", mariaDBType)
		return v
	default:
		// If we reach here, it's an unhandled Go type after scanning.
		log.Printf("Warning: Unhandled Go type '%T' for MariaDB type '%s'. Inserting as is.", v, mariaDBType)
		return v
	}
}
