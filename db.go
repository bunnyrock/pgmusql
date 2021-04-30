package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"

	"github.com/jackc/pgx/v4"

	"github.com/jackc/pgx/v4/pgxpool"
)

type database struct {
	pool            *pgxpool.Pool
	filterOutParams bool
	filterInParams  bool
	muteDbErr       bool
}

// create db connect
func dbNew(dburl string, filterOutParams bool, filterInParams bool, muteDbErr bool) (*database, error) {
	var db database
	var err error

	if db.pool, err = pgxpool.Connect(context.Background(), dburl); err != nil {
		return nil, err
	}

	db.filterOutParams = filterOutParams
	db.filterInParams = filterInParams
	db.muteDbErr = muteDbErr

	return &db, nil
}

// close db connection
func (db *database) close() {
	db.pool.Close()
}

// execute query

var errQueryDBError = errors.New("Database error")

type dbQueryResult struct {
	res   []byte
	total int
	err   error
}

func (db *database) queryChannel(ctx context.Context, q query, params url.Values, limit int) <-chan dbQueryResult {
	res := make(chan dbQueryResult)
	go func() {
		r, t, err := db.query(ctx, q, params, limit)
		res <- dbQueryResult{r, t, err}
	}()
	return res
}

func (db *database) query(ctx context.Context, q query, params url.Values, limit int) (res []byte, total int, err error) {
	// check err
	defer func() {
		if db.muteDbErr && err != nil {
			err = errQueryDBError
		}
	}()

	// prepare query params
	var prms []interface{}
	if prms, err = q.params.prepare(params, db.filterInParams); err != nil {
		return
	}

	// run query
	var rows pgx.Rows
	if rows, err = db.pool.Query(ctx, q.body, prms...); err != nil {
		return
	}
	defer rows.Close()

	// conver db rows to json
	return dbRowsToJSON(rows, db.filterOutParams, q.out, limit)
}

// db row to json converter
func dbRowsToJSON(rows pgx.Rows, filterOutParams bool, outparams dirParamList, limit int) ([]byte, int, error) {
	table := make([]map[string]interface{}, 0)

	i := 0
	for rows.Next() && (limit == 0 || i < limit) {
		trow := make(map[string]interface{}, 0)
		fields := rows.FieldDescriptions()

		val, err := rows.Values()
		if err != nil {
			return nil, 0, err
		}

		for i, column := range fields {
			if filterOutParams {
				if find, _ := outparams.find(string(column.Name)); !find {
					continue
				}
			}
			trow[string(column.Name)] = val[i]
		}

		table = append(table, trow)
		i++
	}

	jsn, err := json.Marshal(table)
	if err != nil {
		return nil, 0, err
	}

	return jsn, len(table), nil
}
