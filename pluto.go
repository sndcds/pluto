package pluto

import (
	_ "context"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/pgxpool"
	"io/ioutil"
	_ "log"
	"os"
)

type Pluto struct {
	Config        Config
	Db            *pgxpool.Pool
	Verbose       bool
	ImageDir      string
	SqlQueryEvent string
}

var Singleton *Pluto

// New creates and initializes a new Pluto instance
func New(configFilePath string, db *pgxpool.Pool, verbose bool) (*Pluto, error) {
	pluto := &Pluto{}
	pluto.Verbose = verbose
	pluto.Db = db

	pluto.Log("loading configuration")
	if err := pluto.loadConfig(configFilePath); err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	pluto.Log("prepare sql")
	if err := pluto.prepareSql(); err != nil {
		return nil, fmt.Errorf("failed to prepare SQL: %w", err)
	}

	Singleton = pluto

	return pluto, nil
}

func (pluto *Pluto) Log(msg string) {
	if pluto.Verbose {
		fmt.Println("pluto:", msg)
	}
}

/*
func (pluto *pluto) Init(configFilePath string) error {
	err := pluto.loadConfig(configFilePath)
	if err != nil {
		panic(err)
	}

	err = pluto.prepareSql()
	if err != nil {
		panic(err)
	}

	pluto.initDB()
	defer pluto.ImageDB.Close()

	return nil
}
*/

func (pluto *Pluto) loadConfig(configFilePath string) error {
	file, err := os.Open(configFilePath)
	if err != nil {
		return err
	}
	defer file.Close()

	bytes, err := ioutil.ReadAll(file)
	if err != nil {
		return err
	}

	err = json.Unmarshal(bytes, &pluto.Config)
	if err != nil {
		return err
	}

	pluto.Config.Print()

	return nil
}

func (pluto *Pluto) prepareSql() error {
	return nil
}

func (pluto *Pluto) RegisterRoutes(rg *gin.RouterGroup, middlewares ...gin.HandlerFunc) {
	group := rg.Group("/image")
	group.GET("/get", getImageHandler)
	group.GET("/file/:file", getFileHandler)
	// protected := group.Group("/", middlewares...)
	// protected.POST("/upload", UploadImageHandler)
}
