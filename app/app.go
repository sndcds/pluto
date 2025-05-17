package app

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"io/ioutil"
	"log"
	"os"
)

type App struct {
	ImageDB *pgxpool.Pool
	UserDB  *pgxpool.Pool
	Config  Config
}

var GApp App

func (app *App) LoadConfig(fileName string) error {
	file, err := os.Open(fileName)
	if err != nil {
		return err
	}
	defer file.Close()

	bytes, err := ioutil.ReadAll(file)
	if err != nil {
		return err
	}

	err = json.Unmarshal(bytes, &app.Config)
	if err != nil {
		return err
	}

	app.Config.Print()
	return nil
}

func (app *App) PrepareSql() error {
	/*
		// Read the SQL file
		content, err := os.ReadFile("queries/queryEvent.sql")
		if err != nil {
			panic(fmt.Errorf("failed to read file: %w", err))
		}

		// Convert to string and replace {{schema}} with actual schema name
		query := string(content)
		query = strings.ReplaceAll(query, "{{schema}}", app.Config.DBSchema)
		app.SqlQueryEvent = query
		fmt.Println(app.SqlQueryEvent)
	*/
	return nil
}

func (app *App) InitDB() error {

	connStr := fmt.Sprintf("postgres://%s:%s@%s:%d/%s", app.Config.User, app.Config.Password, app.Config.Host, app.Config.Port, app.Config.DBName)

	var err error
	app.ImageDB, err = pgxpool.New(context.Background(), connStr)
	if err != nil {
		log.Fatalf("Unable to create connection pool: %v\n", err)
		return err
	}

	fmt.Println("Database connection pool initialized!")
	return nil
}

func (app *App) CloseAllDBs() {
	if app.ImageDB != nil {
		app.ImageDB.Close()
	}
	if app.UserDB != nil {
		app.UserDB.Close()
	}
}
