package pluto

import (
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"strconv"

	_ "github.com/lib/pq"
	"github.com/rwcarlsen/goexif/exif"
	"github.com/rwcarlsen/goexif/tiff"
)

type ImageMetadata struct {
	UserID    string
	License   string
	CreatedBy string
	Copyright string
	AltText   string
	FocusX    float64
	FocusY    float64
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
