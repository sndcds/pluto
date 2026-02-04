package pluto

import (
	_ "context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	_ "log"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/pgxpool"
)

type Pluto struct {
	Config   Config
	Verbose  bool
	DbPool   *pgxpool.Pool
	DbSchema string
}

var PlutoInstance *Pluto

// New creates and initializes a new Pluto instance
func Initialize(configFilePath string, pool *pgxpool.Pool, verbose bool) (*Pluto, error) {
	pluto := &Pluto{}

	pluto.Verbose = verbose
	pluto.DbPool = pool

	pluto.Log("loading configuration")
	if err := pluto.loadConfig(configFilePath); err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	pluto.Log("prepare sql")
	if err := pluto.prepareSql(); err != nil {
		return nil, fmt.Errorf("failed to prepare SQL: %w", err)
	}

	pluto.DbSchema = pluto.Config.DbSchema

	PlutoInstance = pluto

	return pluto, nil
}

func (pluto *Pluto) Log(msg string) {
	if pluto.Verbose {
		fmt.Println("pluto:", msg)
	}
}

func (pluto *Pluto) loadConfig(configFilePath string) error {
	pluto.Config = DefaultConfig()

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

	pluto.Config.PlutoRoute = strings.Trim(pluto.Config.PlutoRoute, "/")
	pluto.Config.Print()

	return nil
}

func (pluto *Pluto) prepareSql() error {
	return nil
}

func (pluto *Pluto) RegisterRoutes(rg *gin.RouterGroup, middlewares ...gin.HandlerFunc) {
	group := rg.Group("/" + pluto.Config.PlutoRoute)
	group.GET("/:id/", getImage)
	group.GET("/file/:file", getFile)
	group.GET("/meta/:context/:contextId/:identifier", getImageMeta)
	group.GET("/cache/:imageId", getImageCache)
}
