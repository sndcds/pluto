package pluto

import (
	_ "github.com/lib/pq"
	"github.com/rwcarlsen/goexif/exif"
	"github.com/rwcarlsen/goexif/tiff"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"strconv"
)

type ImageMetadata struct {
	CreatedBy string
	Copyright string
	License   string
	AltText   string
	FocusX    float64
	FocusY    float64
	UserID    string
}

type exifWalker struct {
	m map[string]string
}

func (w *exifWalker) Walk(name exif.FieldName, tag *tiff.Tag) error {
	w.m[string(name)] = tag.String()
	return nil
}

type Param struct {
	Exist bool
	Value string
}

// Order of parameters (fixed positions)
var paramKeys = []string{
	"mode", "type", "quality", "width", "height",
	"ratio", "focusx", "focusy", "brightness", "contrast", "saturation", // ← 11 parameters
}

var paramIndex = map[string]int{
	"mode": 0, "type": 1, "quality": 2, "width": 3, "height": 4,
	"ratio": 5, "focusx": 6, "focusy": 7, "brightness": 8, "contrast": 9, "saturation": 10,
}

// Short codes mapped by param name
var paramShortCodes = map[string]string{
	"mode":       "m",
	"type":       "t",
	"quality":    "q",
	"width":      "w",
	"height":     "h",
	"ratio":      "r",
	"focusx":     "x",
	"focusy":     "y",
	"brightness": "b",
	"contrast":   "c",
	"saturation": "s",
}

func getOrDefault(param Param, def string) string {
	if param.Exist {
		return param.Value
	}
	return def
}

func atoiOrDefault(param Param, def int) int {
	if param.Exist {
		if v, err := strconv.Atoi(param.Value); err == nil {
			return v
		}
	}
	return def
}

func atoiOrDefaultClamped(param Param, def, min, max int) int {
	if param.Exist {
		if v, err := strconv.Atoi(param.Value); err == nil {
			if v < min {
				return min
			}
			if v > max {
				return max
			}
			return v
		}
	}
	return def
}

func atofOrDefault(param Param, def float64) float64 {
	if param.Exist {
		if v, err := strconv.ParseFloat(param.Value, 64); err == nil {
			return v
		}
	}
	return def
}

/*
func main() {
	configFileName := "config.json" // Default config file name
	if len(os.Args) > 1 {
		configFileName = os.Args[1] // Get config file name from args
	}

	err := pluto.Singleton.LoadConfig(configFileName)
	if err != nil {
		panic(err)
	}

	err = pluto.Singleton.prepareSql()
	if err != nil {
		panic(err)
	}

	pluto.Singleton.initDB()
	defer pluto.Singleton.Db.Close()

	// Create upload dirs
	os.MkdirAll(pluto.Singleton.Config.ImageUploadDir, os.ModePerm)

	// Create a Gin router
	r := gin.Default()
	gin.SetMode(gin.ReleaseMode)

	// Tell Gin where to find HTML templates
	r.LoadHTMLGlob("templates/*")

	// Route to serve upload form
	r.GET("/upload-image", func(c *gin.Context) {
		c.HTML(http.StatusOK, "upload-image.html", nil)
	})

	// Routes
	r.POST("/upload", uploadHandler)
	r.GET("/image", getImageHandler)

	fmt.Println("🚀 Server running at http://localhost:8080/upload-image")
	err = r.Run(":8080")
	if err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}
}
*/
