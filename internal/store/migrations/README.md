# Migrations

One forward-only `*.sql` file per change, named `NNNN_description.sql` and
applied in lexical order. Each runs in its own transaction and is recorded in
`schema_migrations` with the sha256 of its bytes.

A file that has already run is never edited. The recorded sha256 stops the open
if it is, because two databases would otherwise share a ledger and hold
different schemas with nothing to tell them apart. Correcting a mistake means a
new file, or deleting the database while nothing has shipped.
