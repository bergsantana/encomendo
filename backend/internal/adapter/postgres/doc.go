// Package postgres is the Postgres adapter: the GORM connection, the
// transaction manager, the repositories and the embedded SQL migrations.
//
// GORM never creates schema here. AutoMigrate is never called; the schema
// comes only from the SQL files in migrations/.
package postgres
