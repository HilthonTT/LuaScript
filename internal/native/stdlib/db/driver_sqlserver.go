package db

// Microsoft SQL Server / Azure SQL — registers the driver names "sqlserver"
// and "mssql".
//
//	db.open("sqlserver", "sqlserver://user:pass@localhost:1433?database=mydb")
//
// Bind parameters are @p1, @p2, … (db.placeholder reports this).
import _ "github.com/microsoft/go-mssqldb"
