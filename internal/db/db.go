// SPDX-License-Identifier: MIT

// Package db provides a BoltDB-backed persistent data store for HOS entities.
// It handles CRUD operations for pools, objects, users, and keys with generic type support.
package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/filter"
	"github.com/lithammer/fuzzysearch/fuzzy"
	bolt "github.com/timshannon/bolthold"
)

// DB represents a persistent data store backed by BoltDB
type DB struct {
	store *bolt.Store
	log   *slog.Logger
}

// New creates a new database instance in the specified root directory
func New(root string, log *slog.Logger) (*DB, error) {
	log = log.With("lib", "db")

	storeDir := filepath.Join(root, ".db")
	log.Debug("creating db directory", "path", storeDir)
	if err := os.MkdirAll(storeDir, 0o700); err != nil {
		return nil, err
	}

	storePath := filepath.Join(storeDir, "hos.db")
	boltOptions := &bolt.Options{
		Encoder: json.Marshal,
		Decoder: json.Unmarshal,
	}
	log.Debug("opening db file", "path", storePath)
	boltStore, err := bolt.Open(storePath, 0o600, boltOptions)
	if err != nil {
		return nil, err
	}

	return &DB{store: boltStore, log: log}, nil
}

// Close closes the database
func (db *DB) Close() error {
	return db.store.Close()
}

// Create inserts a new entity into the database
func Create[T any](ctx context.Context, db *DB, t *T) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	db.log.Debug("creating "+stn(t), sarg(t)...)

	if err := db.store.Insert(getID(t), t); err != nil {
		if errors.Is(err, bolt.ErrKeyExists) {
			err = hos.ErrExist
		}
		return err
	}

	return nil
}

// Get retrieves an entity by ID from the database
func Get[T any](ctx context.Context, db *DB, id string) (*T, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var result T
	db.log.Debug("getting "+stn(result), "id", id)

	if err := db.store.FindOne(&result, bolt.Where(bolt.Key).Eq(id)); err != nil {
		if errors.Is(err, bolt.ErrNotFound) {
			err = hos.ErrNotExist
		}
		return nil, err
	}

	return &result, nil
}

// Update modifies an existing entity in the database
func Update[T any](ctx context.Context, db *DB, t *T) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	db.log.Debug("updating "+stn(t), sarg(t)...)

	if err := db.store.Update(getID(t), t); err != nil {
		if errors.Is(err, bolt.ErrNotFound) {
			err = hos.ErrNotExist
		}
		return err
	}

	return nil
}

// Delete removes an entity from the database, checking for child dependencies
func Delete[T, K any](ctx context.Context, db *DB, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	var (
		entityType T
		childType  K
	)
	db.log.Debug("deleting "+stn(entityType), "id", id)

	if reflect.ValueOf(entityType).Type().Name() != reflect.ValueOf(childType).Type().Name() {
		count, err := Count[K](ctx, db, id)
		if err != nil {
			return err
		}
		if count > 0 {
			return hos.ErrNotEmpty
		}
	}

	err := db.store.Delete(id, &entityType)
	if errors.Is(err, bolt.ErrNotFound) {
		err = hos.ErrNotExist
	}
	return err
}

// List retrieves entities with optional filtering and sorting
func List[T any](ctx context.Context, db *DB, parentID string, queryFunctions ...QueryFunc) ([]T, error) {
	results := []T{}
	if err := ctx.Err(); err != nil {
		return results, err
	}
	var entityType T
	db.log.Debug(fmt.Sprintf("listing %ss", stn(entityType)), "parent_id", parentID)

	query := listQuery(parentID, &entityType)
	for _, queryFunc := range queryFunctions {
		query = queryFunc(query)
	}

	if err := db.store.Find(&results, query); err != nil {
		return nil, err
	}

	return results, nil
}

// Find performs fuzzy search on entity names
func Find[T any](ctx context.Context, db *DB, searchTerm string) ([]T, error) {
	results := []T{}
	if err := ctx.Err(); err != nil {
		return results, err
	}
	var entityType T
	db.log.Debug(fmt.Sprintf("fuzzy finding %s in %ss", searchTerm, stn(entityType)))
	query := fuzzyFind(searchTerm)(&bolt.Query{}).SortBy("Name")

	if err := db.store.Find(&results, query); err != nil {
		return nil, err
	}

	return results, nil
}

// Count returns the number of entities under a parent
func Count[T any](ctx context.Context, db *DB, parentID string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	var entityType T
	db.log.Debug(fmt.Sprintf("counting %ss", stn(entityType)), "parent_id", parentID)
	return db.store.Count(&entityType, listQuery(parentID, &entityType))
}

// QueryFunc represents a function that modifies database queries
type QueryFunc func(q *bolt.Query) *bolt.Query

// Range limits query results with skip and limit
func Range(start, count int) QueryFunc {
	return func(query *bolt.Query) *bolt.Query {
		return query.Skip(start).Limit(count)
	}
}

// NamePrefix filters entities by name prefix
func NamePrefix(prefix string) QueryFunc {
	return func(query *bolt.Query) *bolt.Query {
		return query.And("Name").MatchFunc(func(name string) (bool, error) {
			return strings.HasPrefix(name, prefix), nil
		})
	}
}

// SortByFields sorts query results by specified fields
func SortByFields(fields ...string) QueryFunc {
	return func(query *bolt.Query) *bolt.Query {
		return query.SortBy(fields...)
	}
}

// Labels filters entities by label key-value pairs
func Labels(labelFilters ...filter.Label) QueryFunc {
	return func(query *bolt.Query) *bolt.Query {
		return query.And("Labels").MatchFunc(func(labels map[string]string) (bool, error) {
			for _, label := range labelFilters {
				value, exists := labels[label.Key]
				if (label.Equal && (!exists || value != label.Value)) ||
					(!label.Equal && exists && value == label.Value) {
					return false, nil
				}
			}
			return true, nil
		})
	}
}

func fuzzyFind(searchTerm string) QueryFunc {
	return func(query *bolt.Query) *bolt.Query {
		return query.And("Name").MatchFunc(func(name string) (bool, error) {
			return fuzzy.Match(searchTerm, name), nil
		})
	}
}

func stn(entity any) string {
	value := reflect.ValueOf(entity)
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}

	return strings.ToLower(value.Type().Name())
}

func sarg(entity any) []any {
	value := reflect.ValueOf(entity)
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}

	typeName := strings.ToLower(value.Type().Name())
	args := []any{}

	if idField := value.FieldByName("ID"); idField.IsValid() {
		args = append(args, typeName+"_id")
		args = append(args, idField.String())
	}

	if nameField := value.FieldByName("Name"); nameField.IsValid() {
		args = append(args, typeName+"_name")
		args = append(args, nameField.String())
	}

	return args
}

func getID(entity any) any {
	value := reflect.ValueOf(entity)
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}

	var idValue any
	for i := 0; i < value.NumField(); i++ {
		fieldType := value.Type().Field(i)
		if fieldType.Name == "ID" {
			idValue = value.Field(i).Interface()
			break
		}
	}

	return idValue
}

func listQuery(parentID string, entity any) *bolt.Query {
	value := reflect.ValueOf(entity)
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}

	parentFieldName := ""
	for i := 0; i < value.NumField(); i++ {
		fieldType := value.Type().Field(i)
		if _, hasIndex := fieldType.Tag.Lookup("boltholdIndex"); hasIndex {
			parentFieldName = fieldType.Name
			break
		}
	}

	query := &bolt.Query{}
	if parentFieldName != "" {
		query = bolt.Where(parentFieldName).Eq(parentID)
	}

	if nameField := value.FieldByName("Name"); nameField.IsValid() {
		query.SortBy("Name")
	}

	return query
}
