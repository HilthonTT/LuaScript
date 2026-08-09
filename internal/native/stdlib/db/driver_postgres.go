package db

// PostgreSQL — registers the driver name "postgres".
//
//	db.open("postgres", "postgres://user:pass@localhost/mydb?sslmode=disable")
//
// Bind parameters are $1, $2, … (db.placeholder reports this).
import _ "github.com/lib/pq"
