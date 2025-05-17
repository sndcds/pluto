package app

import (
	"fmt"
)

// Config holds database configuration details
type Config struct {
	Host              string `json:"host"`
	Port              int    `json:"port"`
	User              string `json:"user"`
	Password          string `json:"password"`
	DBName            string `json:"dbname"`
	DBSchema          string `json:"dbschema"`
	SSLMode           string `json:"sslmode"`
	ImageUploadDir    string `json:"image_upload_dir"`
	ImageCacheDir     string `json:"image_cache_dir"`
	UserDBSchema      string `json:"userdb_shema"`
	UserDBTable       string `json:"userdb_table"`
	UserDBLoginColumn string `json:"userdb_login_column"`
	UserDBHashColumn  string `json:"userdb_hash_column"`
}

func (config Config) Print() {
	fmt.Println("Config")
	fmt.Printf("  Host: %s\n", config.Host)
	fmt.Printf("  Port: %d\n", config.Port)
	fmt.Printf("  User: %s\n", config.User)
	fmt.Printf("  DBName: %s\n", config.DBName)
	fmt.Printf("  DBSchema: %s\n", config.DBSchema)
	fmt.Printf("  SSL mode: %s\n", config.SSLMode)
	fmt.Printf("  ImageUploadDir: %s\n", config.ImageUploadDir)
	fmt.Printf("  ImageCacheDir: %s\n", config.ImageCacheDir)
	fmt.Printf("  UserDBSchema: %s\n", config.UserDBSchema)
	fmt.Printf("  UserDBTable: %s\n", config.UserDBTable)
	fmt.Printf("  UserDBLoginColumn: %s\n", config.UserDBLoginColumn)
	fmt.Printf("  UserDBHashColumn: %s\n", config.UserDBHashColumn)
}
