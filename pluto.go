package pluto

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"io/ioutil"
	"log"
	"os"
)

type Pluto struct {
	ImageDB *pgxpool.Pool
	UserDB  *pgxpool.Pool
	Config  Config
}

var Singleton *Pluto

func (app *Pluto) Init(configFilePath string) error {
	err := app.loadConfig(configFilePath)
	if err != nil {
		panic(err)
	}

	err = app.prepareSql()
	if err != nil {
		panic(err)
	}

	app.initDB()
	defer app.ImageDB.Close()

	return nil
}

func (app *Pluto) loadConfig(configFilePath string) error {
	file, err := os.Open(configFilePath)
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

func (app *Pluto) prepareSql() error {
	/*
		// Read the SQL file
		content, err := os.ReadFile("queries/queryEvent.sql")
		if err != nil {
			panic(fmt.Errorf("failed to read file: %w", err))
		}

		// Convert to string and replace {{schema}} with actual schema name
		query := string(content)
		query = strings.ReplaceAll(query, "{{schema}}", pluto.Config.DBSchema)
		pluto.SqlQueryEvent = query
		fmt.Println(pluto.SqlQueryEvent)
	*/
	return nil
}

func (app *Pluto) initDB() error {

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

func (app *Pluto) closeAllDBs() {
	if app.ImageDB != nil {
		app.ImageDB.Close()
	}
	if app.UserDB != nil {
		app.UserDB.Close()
	}
}

func (app *Pluto) RegisterRoutes(rg *gin.RouterGroup, middlewares ...gin.HandlerFunc) {
	group := rg.Group("/images", middlewares...) // apply Uranus-provided middleware

	group.POST("/", postHandler)
	group.GET("/:id", getHandler)
}

func postHandler(c *gin.Context) {
	// TODO: This is for demo purpose only
}

func getHandler(c *gin.Context) {
	// TODO: This is for demo purpose only
}
