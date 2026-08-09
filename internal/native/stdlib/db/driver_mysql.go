package db

// MySQL and MariaDB — registers the driver name "mysql".
//
//	db.open("mysql", "user:pass@tcp(127.0.0.1:3306)/mydb")
//
// Bind parameters are ?. Note that MySQL's text protocol returns most column
// values as raw bytes; goToLua uses the column type to restore numbers, so a
// SELECT of an INT column still reaches Lua as a number. See coerceColumn.
import _ "github.com/go-sql-driver/mysql"
