package pluto

import "time"

type ImageIdentifierValidator func(string) bool

type ImageRefresherCallback func(entity string, uuids []string) TxFunc

type ImageMeta struct {
	Uuid        *string        `json:"uuid"`
	FileName    *string        `json:"file_name,omitempty"`
	Width       *int           `json:"width,omitempty"`
	Height      *int           `json:"height,omitempty"`
	MimeType    *string        `json:"mime_type,omitempty"`
	AltText     *string        `json:"alt_text,omitempty"`
	Description *string        `json:"description,omitempty"`
	License     *string        `json:"license,omitempty"`
	Exif        map[string]any `json:"exif,omitempty"`
	Expiration  *string        `json:"expiration_date,omitempty"`
	Creator     *string        `json:"creator,omitempty"`
	Copyright   *string        `json:"copyright,omitempty"`
	FocusX      *float64       `json:"focus_x,omitempty"`
	FocusY      *float64       `json:"focus_y,omitempty"`
}

type CacheEntry struct {
	Id        int       `json:"id"`
	Receipt   string    `json:"receipt"`
	ImageId   *int      `json:"image_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	MimeType  *string   `json:"mime_type,omitempty"`
}
