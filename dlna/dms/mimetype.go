package dms

import (
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"path"
	"strings"
)

func init() {
	if err := mime.AddExtensionType(".rmvb", "application/vnd.rn-realmedia-vbr"); err != nil {
		slog.Info("could not register MIME type", "mime_type", "application/vnd.rn-realmedia-vbr", "error", err)
	}
	if err := mime.AddExtensionType(".ogv", "video/ogg"); err != nil {
		slog.Info("could not register MIME type", "mime_type", "video/ogg", "error", err)
	}
	if err := mime.AddExtensionType(".ogg", "audio/ogg"); err != nil {
		slog.Info("could not register MIME type", "mime_type", "audio/ogg", "error", err)
	}
}

// Example: "video/mpeg"
type mimeType string

// IsMedia returns true for media MIME-types
func (mt mimeType) IsMedia() bool {
	return mt.IsVideo() || mt.IsAudio() || mt.IsImage()
}

// IsVideo returns true for video MIME-types
func (mt mimeType) IsVideo() bool {
	return strings.HasPrefix(string(mt), "video/") || mt == "application/vnd.rn-realmedia-vbr"
}

// IsAudio returns true for audio MIME-types
func (mt mimeType) IsAudio() bool {
	return strings.HasPrefix(string(mt), "audio/")
}

// IsImage returns true for image MIME-types
func (mt mimeType) IsImage() bool {
	return strings.HasPrefix(string(mt), "image/")
}

// Returns the group "type", the part before the '/'.
func (mt mimeType) Type() string {
	return strings.SplitN(string(mt), "/", 2)[0]
}

// Returns the string representation of this MIME-type
func (mt mimeType) String() string {
	return string(mt)
}

// MimeTypeByPath determines the MIME-type of file at the given path
func MimeTypeByPath(fsys fs.FS, filePath string) (ret mimeType, err error) {
	ret = mimeTypeByBaseName(path.Base(filePath))
	if ret == "" {
		ret, err = mimeTypeByContent(fsys, filePath)
	}
	if ret == "video/x-msvideo" {
		ret = "video/avi"
	} else if ret == "" {
		ret = "application/octet-stream"
	}
	return
}

// Guess MIME-type from the extension, ignoring ".part".
func mimeTypeByBaseName(name string) mimeType {
	name = strings.TrimSuffix(name, ".part")
	ext := path.Ext(name)
	if ext != "" {
		return mimeType(mime.TypeByExtension(ext))
	}
	return mimeType("")
}

// Guess the MIME-type by analysing the first 512 bytes of the file.
func mimeTypeByContent(fsys fs.FS, path string) (ret mimeType, err error) {
	file, err := fsys.Open(path)
	if err != nil {
		return
	}
	defer file.Close()
	var data [512]byte
	if n, err := file.Read(data[:]); err == nil {
		ret = mimeType(http.DetectContentType(data[:n]))
	}
	return
}
