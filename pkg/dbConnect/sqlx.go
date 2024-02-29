package dbconnect

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	_ "github.com/newrelic/go-agent/v3/integrations/nrpq"
)

func ConnectSqlx(dbConfig DBConfig) (db *sqlx.DB, err error) {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=UTC",
		dbConfig.Host,
		dbConfig.Port,
		dbConfig.Dbuser,
		dbConfig.Dbpassword,
		dbConfig.Dbname,
		dbConfig.Sslmode,
	)
	db, err = sqlx.Connect("nrpostgres", dsn)
	if err != nil {
		return nil, err
	}
	err = db.Ping()
	if err != nil {
		return nil, err
	}
	return
}
