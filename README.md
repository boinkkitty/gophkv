# go-lsm-db

`go-lsm-db` is a small SQL-on-top-of-KV database written in Go. It is not a full SQL engine. It is a teaching-sized system that shows how to layer a relational model and a small expression evaluator on top of a sorted, log-backed key-value store.

## What It Has

- a SQL parser for a compact subset of SQL
- typed rows and schemas
- row-to-key/value encoding
- a log-backed key-value store
- an in-memory sorted index rebuilt from the log on open
- primary-key point lookups
- primary-key range scans
- a tree-walk interpreter for SQL expressions
- tests for KV, row encoding, parser behavior, expression evaluation, and SQL execution

## Project Layout

- `internal/sql`
  - SQL parser and AST
  - schema, row, and cell types
  - SQL execution layer
  - expression evaluator
- `internal/keyval`
  - append-only log
  - sorted in-memory key/value index
  - seek and range iteration
- `internal/utils`
  - internal assertions
- `main.go`
  - small demo program

## Underlying Storage Model

The database stores each row as:

- key = table name + encoded primary-key columns
- value = encoded non-primary-key columns

Because the primary key is embedded in the stored key, row order follows primary-key order. That is what enables efficient point lookups and range scans.

For a table like:

```sql
create table link (
  time int64,
  src string,
  dst string,
  primary key (src, dst)
);
```

and a row like:

```sql
insert into link values (123, 'bob', 'alice');
```

the stored key is based on:

- table = `link`
- primary key = `('bob', 'alice')`

and the stored value contains:

- non-primary-key column `time = 123`

## How the KV Layer Works

The KV store is not a full production LSM tree with memtables, SSTables, compaction, bloom filters, or WAL/manifest separation. It is closer to:

- append every mutation to a log
- rebuild a sorted in-memory index from that log on open
- use binary search for reads
- use bounded iteration for scans

Main characteristics:

- writes are appended to a log
- deletes are tombstones
- recovery replays the log
- the active in-memory state is kept in sorted slices

This gives you some of the shape of LSM-style systems:

- immutable append-oriented writes
- sorted key order
- scan-friendly storage layout

but without the multi-level on-disk compaction machinery of a production LSM.

## Supported SQL Syntax

Supported statements:

- `CREATE TABLE ... PRIMARY KEY (...)`
- `INSERT INTO ... VALUES (...)`
- `SELECT ... FROM ... WHERE ...;`
- `UPDATE ... SET ... WHERE ...;`
- `DELETE FROM ... WHERE ...;`

Supported expression syntax:

- integer literals
- string literals
- column names
- unary `-`
- unary `NOT`
- binary `+`, `-`, `*`, `/`
- comparisons `=`, `!=`, `<>`, `<=`, `>=`, `<`, `>`
- boolean `AND`, `OR`
- tuple expressions like `(a, b)`

## What It Can Execute

The parser is broader than the executor. The executor currently supports `WHERE` clauses only when they map cleanly onto the primary key.

Supported execution patterns:

- full primary-key equality

```sql
select time from link where src = 'bob' and dst = 'alice';
```

- primary-key prefix range conditions

```sql
select time from link where src >= 'b';
select time from link where 'b' <= src;
select time from link where src <= 'z';
```

- tuple primary-key range conditions

```sql
select time from link where (src, dst) >= ('bob', 'alice');
```

- computed `SELECT` expressions

```sql
select a * 4 - b, d + c from t where d = 'b';
```

- computed `UPDATE` assignments

```sql
update t set a = a - b, b = a, c = d + c where d = 'b';
```

## What It Cannot Execute

Not implemented:

- `OR` execution in `WHERE`
- arbitrary non-primary-key predicates
- full scans plus expression filtering
- joins
- grouping and aggregates
- `ORDER BY`
- `LIMIT`
- secondary indexes
- transactions
- query planning

Important distinction:

- parsing can accept `OR`
- execution still rejects unsupported `WHERE` forms with `unimplemented WHERE`

That is because the executor only knows how to reduce a condition into one primary-key lookup or one primary-key range.

## Expression Evaluation

`internal/sql/eval.go` contains a tree-walk interpreter for expression ASTs.

It is used for:

- computed `SELECT` output columns
- computed `UPDATE` right-hand sides
- comparison and boolean expression evaluation support inside the expression layer

## Running

Run the demo:

```bash
go run .
```

Run tests:

```bash
go test ./...
```

## Notes

This project is useful for understanding:

- how SQL rows can be mapped onto KV storage
- why primary-key shape matters for query execution
- how ordered keys enable range scans
- why parsing support and execution support are separate concerns
