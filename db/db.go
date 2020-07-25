package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgtype"
	pgtypeuuid "github.com/jackc/pgtype/ext/gofrs-uuid"
	shopspring "github.com/jackc/pgtype/ext/shopspring-numeric"
	"github.com/jackc/pgtype/pgxtype"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/jackc/pgxutil"
)

type ctxKey int

const (
	_ ctxKey = iota
	configCtxKey
	appDBCtxKey
	sysDBCtxKey
	logDBCtxKey
)

var config *Config
var appDB *pgxpool.Pool
var sysDB *pgxpool.Pool
var logDB *pgxpool.Pool

type Config struct {
	AppConnString string
	AppSchema     string

	SysConnString string
	SysSchema     string

	LogConnString string
	LogSchema     string
}

func (c *Config) SetDerivedDefaults() {
	if c.SysConnString == "" {
		c.SysConnString = c.AppConnString
	}

	if c.LogConnString == "" {
		c.LogConnString = c.AppConnString
	}
}

func connect(ctx context.Context, connString string) (*pgxpool.Pool, error) {
	dbconfig, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database connection string: %v", err)
	}
	dbconfig.AfterConnect = afterConnect(config)

	dbpool, err := pgxpool.ConnectConfig(ctx, dbconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %v", err)
	}

	return dbpool, nil
}

func ConnectApp(ctx context.Context) error {
	if appDB != nil {
		return errors.New("app db already connected")
	}

	db, err := connect(ctx, config.AppConnString)
	if err != nil {
		return fmt.Errorf("failed to connect to app db: %v", err)
	}
	appDB = db

	return nil
}

func ConnectSys(ctx context.Context) error {
	if sysDB != nil {
		return errors.New("sys db already connected")
	}

	if config.SysConnString == config.AppConnString {
		sysDB = appDB
	} else {
		db, err := connect(ctx, config.SysConnString)
		if err != nil {
			return fmt.Errorf("failed to connect to sys db: %v", err)
		}
		sysDB = db
	}

	return nil
}

func ConnectLog(ctx context.Context) error {
	if logDB != nil {
		return errors.New("log db already connected")
	}

	if config.LogConnString == config.LogConnString {
		logDB = appDB
	} else {
		db, err := connect(ctx, config.LogConnString)
		if err != nil {
			return fmt.Errorf("failed to connect to log db: %v", err)
		}
		logDB = db
	}

	return nil
}

func ConnectAll(ctx context.Context) error {
	err := ConnectApp(ctx)
	if err != nil {
		return err
	}

	err = ConnectSys(ctx)
	if err != nil {
		return err
	}

	err = ConnectLog(ctx)
	if err != nil {
		return err
	}

	return nil
}

func afterConnect(config *Config) func(context.Context, *pgx.Conn) error {
	return func(ctx context.Context, conn *pgx.Conn) error {
		searchPath, err := pgxutil.SelectString(ctx, conn, "show search_path")
		if err != nil {
			return fmt.Errorf("failed to get search_path: %v", err)
		}

		searchPath = fmt.Sprintf("%s, %s, %s", QuoteSchema(config.AppSchema), searchPath, QuoteSchema(config.SysSchema))
		_, err = conn.Exec(ctx, fmt.Sprintf("set search_path = %s", searchPath))
		if err != nil {
			return fmt.Errorf("failed to set search_path: %v", err)
		}

		err = registerDataTypes(ctx, conn, config.SysSchema)
		if err != nil {
			return fmt.Errorf("failed to register data types: %v", err)
		}

		return nil
	}
}

func registerDataTypes(ctx context.Context, conn *pgx.Conn, systemSchema string) error {
	conn.ConnInfo().RegisterDataType(pgtype.DataType{
		Value: &pgtypeuuid.UUID{},
		Name:  "uuid",
		OID:   pgtype.UUIDOID,
	})
	conn.ConnInfo().RegisterDataType(pgtype.DataType{
		Value: &shopspring.Numeric{},
		Name:  "numeric",
		OID:   pgtype.NumericOID,
	})

	// TODO - figure out better way to handle custom types that will not exist on initial load. Or maybe remove custom
	// types altogether. Currently, the server needs to be restarted after the initial load.

	dataTypeNames := []string{
		"handler_param",
		"_handler_param",
		"handler",
		"_handler",
		"get_handler_result_row_param",
		"_get_handler_result_row_param",
		"get_handler_result_row",
		"_get_handler_result_row",
	}

	for _, typeName := range dataTypeNames {
		dataType, err := pgxtype.LoadDataType(ctx, conn, conn.ConnInfo(), fmt.Sprintf("%s.%s", QuoteSchema(systemSchema), typeName))
		// if err != nil {
		// 	return err
		// }
		if err == nil {
			conn.ConnInfo().RegisterDataType(dataType)
		}
	}

	return nil
}

func SetConfig(c *Config) {
	if config != nil {
		panic("cannot call SetConfig twice")
	}
	if c == nil {
		panic("c must not be nil")
	}
	config = c
}

func GetConfig(ctx context.Context) *Config {
	v := ctx.Value(configCtxKey)
	if v != nil {
		return v.(*Config)
	}

	if config == nil {
		panic("missing config in ctx and config not set")
	}

	return config
}

func WithConfig(ctx context.Context, c *Config) context.Context {
	return context.WithValue(ctx, configCtxKey, c)
}

type DBConn interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, optionsAndArgs ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, optionsAndArgs ...interface{}) pgx.Row
}

func App(ctx context.Context) DBConn {
	v := ctx.Value(appDBCtxKey)
	if v != nil {
		return v.(DBConn)
	}

	if appDB == nil {
		panic("missing appDB in ctx and appDB not set")
	}

	return appDB
}

func WithApp(ctx context.Context, dbconn DBConn) context.Context {
	return context.WithValue(ctx, appDBCtxKey, dbconn)
}

func Sys(ctx context.Context) DBConn {
	v := ctx.Value(sysDBCtxKey)
	if v != nil {
		return v.(DBConn)
	}

	if sysDB == nil {
		panic("missing sysDB in ctx and sysDB not set")
	}

	return sysDB
}

func WithSys(ctx context.Context, dbconn DBConn) context.Context {
	return context.WithValue(ctx, sysDBCtxKey, dbconn)
}

func Log(ctx context.Context) DBConn {
	v := ctx.Value(logDBCtxKey)
	if v != nil {
		return v.(DBConn)
	}

	if logDB == nil {
		panic("missing logDB in ctx and logDB not set")
	}

	return logDB
}

func WithLog(ctx context.Context, dbconn DBConn) context.Context {
	return context.WithValue(ctx, logDBCtxKey, dbconn)
}
