// Package storage documents the Session Runtime persisted schema. Types
// declared here are not used as GORM models by any query - repository code
// defines its own row-shaped structs local to each workflow/usecase package -
// they exist only to mirror the migrations under migrations/ as readable
// schema documentation.
//
// No tables are defined yet.
package storage
