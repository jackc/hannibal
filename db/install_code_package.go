package db

import (
	"context"
	"io/ioutil"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/tern/migrate"

	_ "github.com/jackc/hannibal/embed/statik"
	statikfs "github.com/rakyll/statik/fs"
)

func InstallCodePackage(ctx context.Context, connString, appSchema, sqlPath string) error {
	cps, err := migrate.LoadCodePackageSource(sqlPath)
	if err != nil {
		return err
	}

	statikFS, err := statikfs.New()
	if err != nil {
		return err
	}

	file, err := statikFS.Open("/app_setup.sql")
	if err != nil {
		return err
	}

	buf, err := ioutil.ReadAll(file)
	if err != nil {
		return err
	}

	appSetupName := "app_setup.sql"

	cps.Schema = appSchema
	cps.Manifest = append([]string{appSetupName}, cps.Manifest...)
	cps.SourceCode[appSetupName] = string(buf)

	conn, err := pgx.Connect(ctx, connString)
	if err != nil {
		return err
	}

	codePackage, err := cps.Compile()
	if err != nil {
		return err
	}

	err = codePackage.Install(ctx, conn, map[string]interface{}{})
	if err != nil {
		return err
	}

	return nil
}
