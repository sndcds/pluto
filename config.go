package pluto

import (
	"fmt"
)

// Config holds database configuration details
type Config struct {
	BaseApiUrl      string `json:"base_api_url"`
	DbHost          string `json:"db_host"`
	DbPort          int    `json:"db_port"`
	DbUser          string `json:"db_user"`
	DbPassword      string `json:"db_password"`
	DbName          string `json:"db_name"`
	DbSchema        string `json:"db_schema"`
	SSLMode         string `json:"ssl_mode"`
	PlutoVerbose    bool   `json:"pluto_verbose"`
	PlutoImageDir   string `json:"pluto_image_dir"`
	PlutoCacheDir   string `json:"pluto_cache_dir"`
	PlutoMaxImagePx int    `json:"pluto_max_image_px"`
}

func (config Config) Print() {
	fmt.Println("pluto Config")
	fmt.Printf("  base_api_url: %s\n", config.BaseApiUrl)
	fmt.Printf("  db_host: %s\n", config.DbHost)
	fmt.Printf("  db_port: %d\n", config.DbPort)
	fmt.Printf("  db_user: %s\n", config.DbUser)
	fmt.Printf("  db_name: %s\n", config.DbName)
	fmt.Printf("  db_schema: %s\n", config.DbSchema)
	fmt.Printf("  ssl_mode: %s\n", config.SSLMode)
	fmt.Printf("  pluto_verbose: %t\n", config.PlutoVerbose)
	fmt.Printf("  pluto_image_dir: %s\n", config.PlutoImageDir)
	fmt.Printf("  pluto_cache_dir: %s\n", config.PlutoCacheDir)
}
